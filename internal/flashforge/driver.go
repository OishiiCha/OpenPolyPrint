package flashforge

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/lucas/openpolyprint/internal/printers"
)

// Driver implements printers.Driver for FlashForge Adventurer 5M / 5M Pro / AD5X
// printers. It uses the HTTP REST API on port 8898 for status, temperature
// control, job control, file upload, and print start. A raw TCP connection on
// port 8899 is used for SendGCode (and as a fallback for move/extrude on
// models without HTTP moveCtrl_cmd / extrudeCtrl_cmd support).
type Driver struct {
	cfg     printers.PrinterConfig
	client  *http.Client
	baseURL string // http://host:8898

	tcpMu      sync.Mutex
	tcpConn    net.Conn
	tcpLast    time.Time
	tcpControl bool // whether M601 control has been acquired

	mu     sync.RWMutex
	cached printers.Status
}

var _ printers.Driver = (*Driver)(nil)

// NewDriver creates a FlashForge driver from a printer config.
// The Host field should be the printer IP (e.g. "192.168.1.42"). If a port is
// omitted, :8898 is assumed for HTTP and :8899 for TCP.
// SerialNumber is the printer serial (e.g. "SNADVA5MXXXXX").
// APIKey is reused to carry the check code (e.g. "12345").
func NewDriver(cfg printers.PrinterConfig) *Driver {
	host := cfg.Host
	if host == "" {
		host = "localhost"
	}
	// Strip any scheme the user may have pasted.
	host = strings.TrimPrefix(host, "http://")
	host = strings.TrimPrefix(host, "https://")
	// Ensure an explicit HTTP port for the REST API. If the user already
	// included a port, keep it; otherwise default to 8898.
	if _, _, err := net.SplitHostPort(host); err != nil {
		host = net.JoinHostPort(host, "8898")
	}
	return &Driver{
		cfg:     cfg,
		client:  &http.Client{Timeout: 10 * time.Second},
		baseURL: "http://" + host,
	}
}

// PrinterID returns the configured printer ID.
func (d *Driver) PrinterID() string { return d.cfg.ID }

// Name returns the configured printer name.
func (d *Driver) Name() string { return d.cfg.Name }

// Type returns the printer type.
func (d *Driver) Type() string { return "flashforge" }

// Connect verifies the printer is reachable and credentials are valid by
// calling /detail.
func (d *Driver) Connect(ctx context.Context) error {
	_, err := d.fetchDetail(ctx)
	if err != nil {
		return fmt.Errorf("flashforge connect: %w", err)
	}
	return nil
}

// Disconnect releases any TCP control session.
func (d *Driver) Disconnect() error {
	d.releaseTCP()
	return nil
}

// --- HTTP API types ---

type apiResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type detailResponse struct {
	apiResponse
	Detail detailPayload `json:"detail"`
}

// detailPayload captures the subset of /detail fields we use. FlashForge
// firmware returns many more; unknown fields are ignored by the JSON decoder.
type detailPayload struct {
	Status           string  `json:"status"`
	PrintProgress    float64 `json:"printProgress"`
	PrintLayer       int     `json:"printLayer"`
	TargetPrintLayer int     `json:"targetPrintLayer"`
	EstimatedTime    float64 `json:"estimatedTime"`
	PrintDuration    int     `json:"printDuration"`
	PrintFileName    string  `json:"printFileName"`
	PlatTemp         float64 `json:"platTemp"`
	PlatTargetTemp   float64 `json:"platTargetTemp"`
	LeftTemp         float64 `json:"leftTemp"`
	RightTemp        float64 `json:"rightTemp"`
	LeftTargetTemp   float64 `json:"leftTargetTemp"`
	RightTargetTemp  float64 `json:"rightTargetTemp"`
	FirmwareVersion  string  `json:"firmwareVersion"`
	Name             string  `json:"name"`
	NozzleCnt        int     `json:"nozzleCnt"`
}

// fetchDetail calls POST /detail and returns the parsed payload.
func (d *Driver) fetchDetail(ctx context.Context) (detailPayload, error) {
	var resp detailResponse
	if err := d.postJSON(ctx, "/detail", map[string]string{
		"serialNumber": d.cfg.SerialNumber,
		"checkCode":    d.cfg.APIKey,
	}, &resp); err != nil {
		return detailPayload{}, err
	}
	if resp.Code != 0 {
		return detailPayload{}, fmt.Errorf("/detail: code %d: %s", resp.Code, resp.Message)
	}
	return resp.Detail, nil
}

// Status polls /detail and maps it to the unified status.
func (d *Driver) Status() (printers.Status, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	det, err := d.fetchDetail(ctx)
	if err != nil {
		s := printers.Status{
			ID:         d.cfg.ID,
			Name:       d.cfg.Name,
			Type:       "flashforge",
			Online:     false,
			State:      "offline",
			StatusText: "Offline",
			UpdatedAt:  time.Now(),
			Error:      err.Error(),
		}
		d.mu.Lock()
		d.cached = s
		d.mu.Unlock()
		return s, nil
	}

	// AD5X / 5M series use rightTemp for the (single) extruder. Fall back to
	// leftTemp for dual-extruder models that report 0 on the right.
	nozzleTemp := det.RightTemp
	targetNozzle := det.RightTargetTemp
	if det.NozzleCnt > 1 && nozzleTemp == 0 && det.LeftTemp != 0 {
		nozzleTemp = det.LeftTemp
		targetNozzle = det.LeftTargetTemp
	}

	s := printers.Status{
		ID:         d.cfg.ID,
		Name:       d.cfg.Name,
		Type:       "flashforge",
		Online:     true,
		State:      det.Status,
		StatusText: humanizeState(det.Status),
		Temps: printers.Temps{
			Nozzle:       nozzleTemp,
			Bed:          det.PlatTemp,
			TargetNozzle: targetNozzle,
			TargetBed:    det.PlatTargetTemp,
		},
		Progress:    int(det.PrintProgress * 100),
		CurrentFile: det.PrintFileName,
		UpdatedAt:   time.Now(),
		LayerNum:    det.PrintLayer,
		LayerCount:  det.TargetPrintLayer,
	}
	if det.EstimatedTime > 0 {
		s.RemainingTime = formatDurationSeconds(int64(det.EstimatedTime))
	}

	d.mu.Lock()
	d.cached = s
	d.mu.Unlock()
	return s, nil
}

// PausePrint pauses the current print via jobCtl_cmd.
func (d *Driver) PausePrint(ctx context.Context) error {
	return d.sendControl(ctx, "jobCtl_cmd", map[string]string{
		"jobID":  "",
		"action": "pause",
	})
}

// StopPrint cancels the current print via jobCtl_cmd.
func (d *Driver) StopPrint(ctx context.Context) error {
	return d.sendControl(ctx, "jobCtl_cmd", map[string]string{
		"jobID":  "",
		"action": "cancel",
	})
}

// Home homes all axes. AD5X/5M support homingCtl_cmd over HTTP; fall back to
// TCP G28 for older firmware.
func (d *Driver) Home(ctx context.Context) error {
	if err := d.sendControl(ctx, "homingCtrl_cmd", nil); err == nil {
		return nil
	}
	return d.sendTCPGCode(ctx, "G28")
}

// Preheat sets nozzle and bed target temperatures.
func (d *Driver) Preheat(ctx context.Context, nozzle, bed float64) error {
	return d.sendControl(ctx, "temperatureCtl_cmd", map[string]float64{
		"rightNozzle": nozzle,
		"platform":    bed,
	})
}

// Cooldown turns off nozzle and bed heaters (-100 = off).
func (d *Driver) Cooldown(ctx context.Context) error {
	return d.sendControl(ctx, "temperatureCtl_cmd", map[string]float64{
		"rightNozzle": -100,
		"platform":    -100,
	})
}

// AutoLevel triggers bed-leveling calibration.
func (d *Driver) AutoLevel(ctx context.Context) error {
	return d.sendControl(ctx, "calibration_cmd", map[string]string{
		"levelingDetection":     "open",
		"vibrationCompensation": "close",
	})
}

// SendGCode sends a raw G-code command via the TCP API on port 8899.
func (d *Driver) SendGCode(ctx context.Context, command string) error {
	return d.sendTCPGCode(ctx, command)
}

// MoveAxis moves an axis by a relative distance. Uses moveCtrl_cmd over HTTP
// (AD5X/5M) and falls back to TCP relative G-code.
func (d *Driver) MoveAxis(ctx context.Context, axis string, distance float64, speed float64) error {
	axisLower := strings.ToLower(axis)
	if err := d.sendControl(ctx, "moveCtrl_cmd", map[string]any{
		"axis":  axisLower,
		"delta": distance,
	}); err == nil {
		return nil
	}
	// Fallback: TCP relative G-code.
	return d.sendTCPGCode(ctx, fmt.Sprintf("G91\nG0 %s%.2f F%.0f\nG90", strings.ToUpper(axis), distance, speed))
}

// SetNozzleTemp sets the nozzle target temperature.
func (d *Driver) SetNozzleTemp(ctx context.Context, temp float64) error {
	return d.sendControl(ctx, "temperatureCtl_cmd", map[string]float64{
		"rightNozzle": temp,
	})
}

// SetBedTemp sets the bed target temperature.
func (d *Driver) SetBedTemp(ctx context.Context, temp float64) error {
	return d.sendControl(ctx, "temperatureCtl_cmd", map[string]float64{
		"platform": temp,
	})
}

// Extrude extrudes (positive) or retracts (negative) filament. Uses
// extrudeCtrl_cmd over HTTP (AD5X/5M) and falls back to TCP relative G-code.
func (d *Driver) Extrude(ctx context.Context, amount float64, feedrate float64) error {
	if err := d.sendControl(ctx, "extrudeCtrl_cmd", map[string]float64{
		"delta": amount,
	}); err == nil {
		return nil
	}
	return d.sendTCPGCode(ctx, fmt.Sprintf("G91\nG0 E%.2f F%.0f\nG90", amount, feedrate))
}

// UploadGCode uploads a file via /uploadGcode (multipart/form-data).
func (d *Driver) UploadGCode(ctx context.Context, filename string, data []byte, progress func(sent, total int)) error {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("gcodeFile", filepath.Base(filename))
	if err != nil {
		return fmt.Errorf("create form file: %w", err)
	}
	if _, err := part.Write(data); err != nil {
		return fmt.Errorf("write file data: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("close multipart writer: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", d.baseURL+"/uploadGcode", &buf)
	if err != nil {
		return fmt.Errorf("create upload request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("serialNumber", d.cfg.SerialNumber)
	req.Header.Set("checkCode", d.cfg.APIKey)
	req.Header.Set("fileSize", strconv.Itoa(len(data)))
	req.Header.Set("printNow", "false")
	req.Header.Set("levelingBeforePrint", "false")

	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("upload request: %w", err)
	}
	defer resp.Body.Close()
	var api apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&api); err != nil {
		return fmt.Errorf("upload decode: %w", err)
	}
	if api.Code != 0 {
		return fmt.Errorf("upload failed: code %d: %s", api.Code, api.Message)
	}
	if progress != nil {
		progress(len(data), len(data))
	}
	return nil
}

// StartPrint begins printing a file already on the printer via /printGcode.
func (d *Driver) StartPrint(ctx context.Context, filename string) error {
	body := map[string]any{
		"serialNumber":        d.cfg.SerialNumber,
		"checkCode":           d.cfg.APIKey,
		"fileName":            filepath.Base(filename),
		"levelingBeforePrint": false,
	}
	var resp apiResponse
	if err := d.postJSON(ctx, "/printGcode", body, &resp); err != nil {
		return err
	}
	if resp.Code != 0 {
		return fmt.Errorf("/printGcode: code %d: %s", resp.Code, resp.Message)
	}
	return nil
}

// --- HTTP helpers ---

// sendControl posts a /control command with the given cmd name and args.
func (d *Driver) sendControl(ctx context.Context, cmd string, args any) error {
	payload := map[string]any{"cmd": cmd}
	if args != nil {
		payload["args"] = args
	}
	body := map[string]any{
		"serialNumber": d.cfg.SerialNumber,
		"checkCode":    d.cfg.APIKey,
		"payload":      payload,
	}
	var resp apiResponse
	if err := d.postJSON(ctx, "/control", body, &resp); err != nil {
		return err
	}
	if resp.Code != 0 {
		return fmt.Errorf("/control %s: code %d: %s", cmd, resp.Code, resp.Message)
	}
	return nil
}

// postJSON sends a JSON POST and decodes the response envelope.
func (d *Driver) postJSON(ctx context.Context, path string, body any, out any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", d.baseURL+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	// FlashForge always returns HTTP 200; the real result is in the JSON code.
	if out != nil {
		if err := json.Unmarshal(b, out); err != nil {
			return fmt.Errorf("POST %s: decode: %w (body: %s)", path, err, string(b))
		}
	}
	return nil
}

// --- TCP API (port 8899) ---

// tcpHost returns the host:8899 address for the TCP protocol.
func (d *Driver) tcpHost() string {
	u, err := url.Parse(d.baseURL)
	if err != nil {
		return ""
	}
	host := u.Hostname()
	if _, _, e := net.SplitHostPort(host); e != nil {
		host = net.JoinHostPort(host, "8899")
	} else {
		// Replace the HTTP port with 8899.
		host = net.JoinHostPort(u.Hostname(), "8899")
	}
	return host
}

// ensureTCP opens (or reuses) a TCP control session on port 8899. FlashForge
// requires M601 S1 to acquire control before most commands are accepted.
func (d *Driver) ensureTCP(ctx context.Context) error {
	d.tcpMu.Lock()
	defer d.tcpMu.Unlock()

	if d.tcpConn != nil {
		// Refresh the session if it has been idle for a while.
		if time.Since(d.tcpLast) > 15*time.Second {
			if err := d.sendTCPRaw("M27"); err != nil {
				d.closeTCP()
			} else {
				d.tcpLast = time.Now()
			}
		}
		if d.tcpConn != nil && d.tcpControl {
			return nil
		}
	}

	addr := d.tcpHost()
	if addr == "" {
		return fmt.Errorf("tcp: no host configured")
	}
	dialer := net.Dialer{Timeout: 5 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("tcp dial %s: %w", addr, err)
	}
	d.tcpConn = conn
	// Acquire control.
	if err := d.sendTCPRaw("M601 S1"); err != nil {
		d.closeTCP()
		return fmt.Errorf("tcp M601: %w", err)
	}
	d.tcpControl = true
	d.tcpLast = time.Now()
	return nil
}

// sendTCPGCode sends a G-code command (possibly multi-line) over the TCP
// session. Each line is prefixed with "~" and terminated with "\r\n".
func (d *Driver) sendTCPGCode(ctx context.Context, command string) error {
	if err := d.ensureTCP(ctx); err != nil {
		return err
	}
	d.tcpMu.Lock()
	defer d.tcpMu.Unlock()
	for _, line := range strings.Split(command, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if err := d.sendTCPRaw(line); err != nil {
			d.closeTCP()
			return fmt.Errorf("tcp send %q: %w", line, err)
		}
	}
	return nil
}

// sendTCPRaw writes a single command (without the ~ prefix) and reads the
// response, returning an error if it does not end with "ok".
func (d *Driver) sendTCPRaw(command string) error {
	if d.tcpConn == nil {
		return fmt.Errorf("tcp: not connected")
	}
	full := "~" + command + "\r\n"
	if _, err := d.tcpConn.Write([]byte(full)); err != nil {
		return err
	}
	// Read until we see "ok" on its own line or hit a short deadline.
	_ = d.tcpConn.SetReadDeadline(time.Now().Add(5 * time.Second))
	reader := bufio.NewReader(d.tcpConn)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return err
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "ok" {
			return nil
		}
		if strings.HasPrefix(trimmed, "Error") {
			return fmt.Errorf("printer error: %s", trimmed)
		}
	}
}

func (d *Driver) closeTCP() {
	if d.tcpConn != nil {
		_ = d.tcpConn.Close()
		d.tcpConn = nil
	}
	d.tcpControl = false
}

// releaseTCP sends M602 to release control before closing the connection.
func (d *Driver) releaseTCP() {
	d.tcpMu.Lock()
	defer d.tcpMu.Unlock()
	if d.tcpConn == nil {
		return
	}
	if d.tcpControl {
		_ = d.tcpConn.SetWriteDeadline(time.Now().Add(2 * time.Second))
		_, _ = d.tcpConn.Write([]byte("~M602\r\n"))
	}
	d.closeTCP()
}

// --- Helpers ---

func humanizeState(state string) string {
	switch state {
	case "ready":
		return "Idle"
	case "busy":
		return "Busy"
	case "calibrate_doing":
		return "Calibrating"
	case "error":
		return "Error"
	case "heating":
		return "Heating"
	case "printing":
		return "Printing"
	case "pausing":
		return "Printing"
	case "pause":
		return "Paused"
	case "canceling":
		return "Stopping"
	case "cancel":
		return "Idle"
	case "completed":
		return "Finished"
	case "downloading", "sending", "unzipping", "cloud_slicing":
		return "Busy"
	}
	if state == "" {
		return "Idle"
	}
	log.Printf("[flashforge] unknown state: %q", state)
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
