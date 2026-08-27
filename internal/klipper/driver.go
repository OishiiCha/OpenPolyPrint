package klipper

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/lucas/openpolyprint/internal/printers"
)

// Driver implements printers.Driver over the Moonraker HTTP API.
type Driver struct {
	cfg     printers.PrinterConfig
	client  *http.Client
	baseURL string

	mu     sync.RWMutex
	cached printers.Status
}

var _ printers.Driver = (*Driver)(nil)

// NewDriver creates a Klipper/Moonraker driver from a printer config.
// The Host field should include the scheme and port, e.g. "http://192.168.1.50:7125".
// If no scheme is given, http:// is assumed.
func NewDriver(cfg printers.PrinterConfig) *Driver {
	host := cfg.Host
	if host == "" {
		host = "http://localhost:7125"
	}
	if !strings.HasPrefix(host, "http://") && !strings.HasPrefix(host, "https://") {
		host = "http://" + host
	}
	host = strings.TrimRight(host, "/")

	return &Driver{
		cfg:     cfg,
		client:  &http.Client{Timeout: 10 * time.Second},
		baseURL: host,
	}
}

// PrinterID returns the configured printer ID.
func (d *Driver) PrinterID() string { return d.cfg.ID }

// Name returns the configured printer name.
func (d *Driver) Name() string { return d.cfg.Name }

// Type returns the printer type.
func (d *Driver) Type() string { return "klipper" }

// Connect verifies the Moonraker server is reachable.
func (d *Driver) Connect(ctx context.Context) error {
	// Moonraker server.info endpoint
	var info struct {
		Result struct {
			Version     string `json:"version"`
			KlippyState string `json:"klippy_state"`
		} `json:"result"`
	}
	if err := d.getJSON(ctx, "/server/info", &info); err != nil {
		return fmt.Errorf("moonraker connect: %w", err)
	}
	return nil
}

// Disconnect is a no-op for the HTTP-based driver.
func (d *Driver) Disconnect() error { return nil }

// Status fetches the current printer state from Moonraker.
func (d *Driver) Status() (printers.Status, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Query printer.info for state + filename
	var printerInfo struct {
		Result struct {
			State        string `json:"state"`
			StateMessage string `json:"state_message"`
			Filename     string `json:"print_stats"`
		} `json:"result"`
	}
	// Moonraker printer.info returns state, and print_stats is separate
	var infoResp struct {
		Result struct {
			State        string `json:"state"`
			StateMessage string `json:"state_message"`
		} `json:"result"`
	}
	if err := d.getJSON(ctx, "/printer/info", &infoResp); err != nil {
		d.mu.Lock()
		d.cached = printers.Status{
			ID:         d.cfg.ID,
			Name:       d.cfg.Name,
			Type:       "klipper",
			Online:     false,
			State:      "offline",
			StatusText: "Offline",
			UpdatedAt:  time.Now(),
			Error:      err.Error(),
		}
		s := d.cached
		d.mu.Unlock()
		return s, nil // return cached/offline status, not error
	}

	// Query printer.objects.query for heaters + print_stats
	var objectsResp struct {
		Result struct {
			Status struct {
				Extruder struct {
					Temperature float64 `json:"temperature"`
					Target      float64 `json:"target"`
				} `json:"extruder"`
				HeaterBed struct {
					Temperature float64 `json:"temperature"`
					Target      float64 `json:"target"`
				} `json:"heater_bed"`
				PrintStats struct {
					State         string  `json:"state"`
					Filename      string  `json:"filename"`
					TotalDuration float64 `json:"total_duration"`
					PrintDuration float64 `json:"print_duration"`
					FilamentUsed  float64 `json:"filament_used"`
					Progress      float64 `json:"progress"`
				} `json:"print_stats"`
				VirtualSDCard struct {
					Progress     float64 `json:"progress"`
					IsActive     bool    `json:"is_active"`
					FilePosition float64 `json:"file_position"`
				} `json:"virtual_sdcard"`
				LayerInfo struct {
					Layer       int     `json:"layer"`
					TotalLayers int     `json:"total_layers"`
					Height      float64 `json:"height"`
					TotalHeight float64 `json:"total_height"`
				} `json:"print_stats"` // may not always be present
			} `json:"status"`
		} `json:"result"`
	}

	heatersQuery := url.Values{}
	heatersQuery.Set("heaters", "extruder,heater_bed")
	heatersQuery.Set("print_stats", "")
	heatersQuery.Set("virtual_sdcard", "")
	if err := d.getJSON(ctx, "/printer/objects/query?"+heatersQuery.Encode(), &objectsResp); err != nil {
		log.Printf("[klipper] objects query failed: %v", err)
	}

	st := objectsResp.Result.Status
	ps := st.PrintStats

	s := printers.Status{
		ID:         d.cfg.ID,
		Name:       d.cfg.Name,
		Type:       "klipper",
		Online:     true,
		State:      infoResp.Result.State,
		StatusText: humanizeKlipperState(infoResp.Result.State, ps.State),
		Temps: printers.Temps{
			Nozzle:       st.Extruder.Temperature,
			Bed:          st.HeaterBed.Temperature,
			TargetNozzle: st.Extruder.Target,
			TargetBed:    st.HeaterBed.Target,
		},
		Progress:    int(ps.Progress * 100),
		CurrentFile: ps.Filename,
		UpdatedAt:   time.Now(),
	}

	// Remaining time estimate
	if ps.Progress > 0 && ps.PrintDuration > 0 {
		remaining := ps.PrintDuration / ps.Progress * (1 - ps.Progress)
		s.RemainingTime = formatDurationSeconds(int64(remaining))
	}

	// Layer info (if available from display_status or gcode move)
	if st.LayerInfo.TotalLayers > 0 {
		s.LayerNum = st.LayerInfo.Layer
		s.LayerCount = st.LayerInfo.TotalLayers
	}

	// Try to get layer info from display_status if not in print_stats
	if s.LayerCount == 0 {
		var displayResp struct {
			Result struct {
				Status struct {
					DisplayStatus struct {
						Progress float64 `json:"progress"`
					} `json:"display_status"`
				} `json:"status"`
			} `json:"result"`
		}
		displayQuery := url.Values{}
		displayQuery.Set("display_status", "")
		if err := d.getJSON(ctx, "/printer/objects/query?"+displayQuery.Encode(), &displayResp); err == nil {
			if displayResp.Result.Status.DisplayStatus.Progress > 0 {
				s.Progress = int(displayResp.Result.Status.DisplayStatus.Progress * 100)
			}
		}
	}

	d.mu.Lock()
	d.cached = s
	d.mu.Unlock()

	_ = printerInfo // unused fallback
	return s, nil
}

// PausePrint pauses the current print.
func (d *Driver) PausePrint(ctx context.Context) error {
	return d.postGCode(ctx, "PAUSE")
}

// StopPrint cancels the current print.
func (d *Driver) StopPrint(ctx context.Context) error {
	return d.postGCode(ctx, "CANCEL_PRINT")
}

// Home sends a G28 home-all command.
func (d *Driver) Home(ctx context.Context) error {
	return d.postGCode(ctx, "G28")
}

// Preheat sets nozzle and bed target temperatures.
func (d *Driver) Preheat(ctx context.Context, nozzle, bed float64) error {
	cmd := fmt.Sprintf("M104 S%.0f\nM140 S%.0f", nozzle, bed)
	return d.postGCode(ctx, cmd)
}

// Cooldown turns off nozzle and bed heaters.
func (d *Driver) Cooldown(ctx context.Context) error {
	return d.postGCode(ctx, "M104 S0\nM140 S0")
}

// AutoLevel runs bed leveling (G29 or BED_LEVEL_CALIBRATE macro).
func (d *Driver) AutoLevel(ctx context.Context) error {
	// Try BED_LEVEL_CALIBRATE macro first, fall back to G29
	if err := d.postGCode(ctx, "BED_LEVEL_CALIBRATE"); err != nil {
		return d.postGCode(ctx, "G29")
	}
	return nil
}

// SendGCode sends a raw G-code command to the printer.
func (d *Driver) SendGCode(ctx context.Context, command string) error {
	return d.postGCode(ctx, command)
}

// UploadGCode uploads a G-code file to Moonraker's gcodes directory.
func (d *Driver) UploadGCode(ctx context.Context, filename string, data []byte) error {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("file", filepath.Base(filename))
	if err != nil {
		return fmt.Errorf("create form file: %w", err)
	}
	if _, err := part.Write(data); err != nil {
		return fmt.Errorf("write file data: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("close multipart writer: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", d.baseURL+"/server/files/upload", &buf)
	if err != nil {
		return fmt.Errorf("create upload request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if d.cfg.APIKey != "" {
		req.Header.Set("X-Api-Key", d.cfg.APIKey)
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("upload request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upload failed: %d %s", resp.StatusCode, string(body))
	}
	return nil
}

// StartPrint begins printing a previously uploaded file.
func (d *Driver) StartPrint(ctx context.Context, filename string) error {
	body := map[string]string{"filename": filepath.Base(filename)}
	return d.postJSON(ctx, "/printer/print/start", body)
}

// --- HTTP helpers ---

func (d *Driver) getJSON(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, "GET", d.baseURL+path, nil)
	if err != nil {
		return err
	}
	if d.cfg.APIKey != "" {
		req.Header.Set("X-Api-Key", d.cfg.APIKey)
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GET %s: %d %s", path, resp.StatusCode, string(body))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (d *Driver) postJSON(ctx context.Context, path string, body any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", d.baseURL+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if d.cfg.APIKey != "" {
		req.Header.Set("X-Api-Key", d.cfg.APIKey)
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("POST %s: %d %s", path, resp.StatusCode, string(b))
	}
	return nil
}

func (d *Driver) postGCode(ctx context.Context, gcode string) error {
	body := map[string]string{"script": gcode}
	return d.postJSON(ctx, "/printer/gcode/script", body)
}

// --- Helpers ---

func humanizeKlipperState(klippyState, printState string) string {
	switch printState {
	case "printing":
		return "Printing"
	case "paused":
		return "Paused"
	case "complete":
		return "Finished"
	case "cancelled", "error", "standby":
		if printState == "standby" {
			return "Idle"
		}
		return "Idle"
	}
	// Fall back to klippy state
	switch klippyState {
	case "ready":
		return "Idle"
	case "disconnected":
		return "Offline"
	case "shutdown":
		return "Error"
	}
	return "Idle"
}

func formatDurationSeconds(s int64) string {
	if s <= 0 {
		return "—"
	}
	h := s / 3600
	m := (s % 3600) / 60
	if h > 0 {
		return fmt.Sprintf("%dh %dm", h, m)
	}
	if m > 0 {
		return fmt.Sprintf("%dm", m)
	}
	return fmt.Sprintf("%ds", s)
}
