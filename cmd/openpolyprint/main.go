package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/lucas/openpolyprint/internal/anker"
	"github.com/lucas/openpolyprint/internal/anker/proto/config"
	"github.com/lucas/openpolyprint/internal/cameras"
	"github.com/lucas/openpolyprint/internal/gcode"
	"github.com/lucas/openpolyprint/internal/history"
	"github.com/lucas/openpolyprint/internal/integrations"
	"github.com/lucas/openpolyprint/internal/logstore"
	"github.com/lucas/openpolyprint/internal/pi"
	"github.com/lucas/openpolyprint/internal/printers"
)

type headerTransport struct {
	base http.RoundTripper
}

func (t *headerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", "python-requests/2.31.0")
	}
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "application/json, */*;q=0.8")
	}
	return t.base.RoundTrip(req)
}

var manualPrinters []printers.PrinterConfig

func buildManager(cfg *config.Config) *printers.Manager {
	var drivers []printers.Driver
	if cfg != nil {
		for _, p := range cfg.Printers {
			drivers = append(drivers, anker.NewDriver(p, cfg.Account))
		}
	}
	for _, p := range manualPrinters {
		drivers = append(drivers, printers.NewStaticDriver(p))
	}
	return printers.NewManager(drivers)
}

func loadManualPrinters(path string) []printers.PrinterConfig {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var out []printers.PrinterConfig
	if err := json.Unmarshal(data, &out); err != nil {
		log.Printf("manual printers load: %v", err)
		return nil
	}
	return out
}

func saveManualPrinters(path string, list []printers.PrinterConfig) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}

// autoRecordConfig is the subset of settings used for automatic recording.
type autoRecordConfig struct {
	Enabled  bool    `json:"enabled"`
	Mode     string  `json:"mode"`
	Interval float64 `json:"interval"`
}

func loadAutoRecord(path string) autoRecordConfig {
	var out autoRecordConfig
	data, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	var cfg struct {
		AutoRecord autoRecordConfig `json:"autoRecord"`
	}
	if err := json.Unmarshal(data, &cfg); err == nil {
		out = cfg.AutoRecord
	}
	return out
}

// loadSlicerTarget reads the configured default printer for slicer uploads.
func loadSlicerTarget(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var cfg struct {
		SlicerTarget string `json:"slicerTarget"`
	}
	_ = json.Unmarshal(data, &cfg)
	return cfg.SlicerTarget
}

func safeName(s string) string {
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.ReplaceAll(s, "\\", "_")
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.TrimSuffix(s, ".gcode")
	s = strings.TrimSuffix(s, ".mkv")
	s = strings.TrimSuffix(s, ".avi")
	return s
}

func startAutoRecord(cameraMgr *cameras.Manager, printerID, printerName, file, mode string, interval float64, auto map[string]bool) {
	if cameraMgr == nil {
		return
	}
	timestamp := time.Now().Format("20060102-150405")
	filename := fmt.Sprintf("%s_%s_%s.mkv", safeName(printerName), safeName(file), timestamp)
	for _, cam := range cameraMgr.Store().GetCameras() {
		if cam.PrinterID != printerID {
			continue
		}
		if cam.Type != "usb" && cam.Type != "mipi" {
			continue
		}
		streamer := cameraMgr.Streamers().GetStream(cam.ID)
		if streamer == nil {
			continue
		}
		if cameraMgr.Records().IsRecording(cam.ID) || cameraMgr.Timelapses().IsRecording(cam.ID) {
			continue
		}
		var err error
		if mode == "timelapse" {
			_, err = cameraMgr.Timelapses().Start(cam.ID, streamer, filename, interval)
		} else {
			_, err = cameraMgr.Records().Start(cam.ID, streamer, filename)
		}
		if err == nil {
			auto[cam.ID] = true
			log.Printf("[auto-record] started %s on camera %s", mode, cam.ID)
		} else {
			log.Printf("[auto-record] camera %s start failed: %v", cam.ID, err)
		}
	}
}

func stopAutoRecord(cameraMgr *cameras.Manager, printerID string, auto map[string]bool) {
	if cameraMgr == nil {
		return
	}
	for _, cam := range cameraMgr.Store().GetCameras() {
		if cam.PrinterID != printerID || !auto[cam.ID] {
			continue
		}
		delete(auto, cam.ID)
		if cameraMgr.Records().IsRecording(cam.ID) {
			go func(id string) { cameraMgr.Records().Stop(id) }(cam.ID)
		}
		if cameraMgr.Timelapses().IsRecording(cam.ID) {
			go func(id string) { cameraMgr.Timelapses().Stop(id) }(cam.ID)
		}
	}
}

// trackHistory watches printer statuses, records finished prints, and triggers auto-recording.
func trackHistory(ctx context.Context, mgr *atomic.Pointer[printers.Manager], cameraMgr *cameras.Manager, store *history.Store, settingsFile string, intgMgr *integrations.Manager) {
	last := map[string]printers.Status{}
	started := map[string]time.Time{}
	autoRecordings := map[string]bool{}
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m := mgr.Load()
			if m == nil {
				continue
			}
			cfg := loadAutoRecord(settingsFile)
			for _, s := range m.Statuses() {
				prev, hasPrev := last[s.ID]
				if s.StatusText == "Printing" {
					if _, ok := started[s.ID]; !ok {
						started[s.ID] = time.Now()
					}
					if cfg.Enabled && (!hasPrev || prev.StatusText != "Printing") {
						startAutoRecord(cameraMgr, s.ID, s.Name, s.CurrentFile, cfg.Mode, cfg.Interval, autoRecordings)
					}
				} else if hasPrev && prev.StatusText == "Printing" && s.StatusText != "Printing" {
					stopAutoRecord(cameraMgr, s.ID, autoRecordings)
					start := started[s.ID]
					if start.IsZero() {
						start = s.UpdatedAt
					}
					file := prev.CurrentFile
					if file == "" {
						file = s.CurrentFile
					}
					if file == "" {
						file = "unknown"
					}
					var result string
					switch s.StatusText {
					case "Finished":
						result = "Success"
					case "Idle":
						// If the previous status was Printing at 100% progress,
						// the print completed even if the driver reports Idle
						// (some printers keep the file loaded after completion).
						if prev.Progress >= 100 || s.CurrentFile == "" {
							result = "Success"
						} else {
							result = "Cancelled"
						}
					case "Error", "Offline":
						result = "Failed"
					default:
						// Paused or transient — don’t record yet
						last[s.ID] = s
						continue
					}
					store.Add(s.Name, file, result, start, time.Now())
					delete(started, s.ID)
				}
				last[s.ID] = s
			}
		}
	}
}

// findDistDir looks for a built frontend/frontend dist directory in multiple
// likely locations: next to the binary, next to the binary's parent, and CWD.
func findDistDir() string {
	candidates := []string{}
	if ex, err := os.Executable(); err == nil {
		exDir := filepath.Dir(ex)
		candidates = append(candidates,
			filepath.Join(exDir, "frontend", "dist"),
			filepath.Join(exDir, "..", "frontend", "dist"),
		)
	}
	candidates = append(candidates, filepath.Join("frontend", "dist"))
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && info.IsDir() {
			return c
		}
	}
	return ""
}

func main() {
	var (
		dataDir = flag.String("data-dir", "", "directory that holds ankerctl default.json (default: platform config dir/ankerctl)")
		addr    = flag.String("addr", ":80", "http listen address for the api")
	)
	flag.Parse()

	logStore := logstore.New(2000)
	log.SetOutput(io.MultiWriter(os.Stderr, logStore))

	// The Anker cloud endpoints currently reject Go's HTTP/2 handshake with a 502,
	// and they require a non-Go User-Agent, so force HTTP/1.1 and python-requests
	// headers for every outbound request.
	http.DefaultTransport = &headerTransport{
		base: &http.Transport{
			Proxy:        http.ProxyFromEnvironment,
			TLSNextProto: make(map[string]func(string, *tls.Conn) http.RoundTripper),
		},
	}

	// OPENPOLYPRINT_DATA_DIR lets Docker/hosters keep config on a persistent
	// volume (e.g. /data/openpolyprint) separate from the image/container.
	var settingsDir string
	if envDataDir := os.Getenv("OPENPOLYPRINT_DATA_DIR"); envDataDir != "" {
		settingsDir = filepath.Join(envDataDir, "openpolyprint")
		if *dataDir == "" {
			*dataDir = filepath.Join(envDataDir, "ankerctl")
		}
	} else {
		userConfigDir, err := os.UserConfigDir()
		if err != nil {
			log.Fatalf("user config dir: %v", err)
		}
		settingsDir = filepath.Join(userConfigDir, "openpolyprint")
	}
	settingsFile := filepath.Join(settingsDir, "settings.json")
	gcodeDir := filepath.Join(settingsDir, "gcode")
	manualPrinters = loadManualPrinters(filepath.Join(settingsDir, "printers.json"))

	gcodeStore, err := gcode.NewStore(gcodeDir)
	if err != nil {
		log.Fatalf("gcode store: %v", err)
	}

	historyStore := history.NewStore(settingsDir)

	cameraMgr := cameras.NewManager(settingsDir)
	piMgr := pi.NewManagerGroup(settingsDir)

	intgMgr := integrations.NewManager()
	if data, err := os.ReadFile(settingsFile); err == nil {
		var openpolyprintSettings struct {
			Integrations map[string]struct {
				Enabled bool              `json:"enabled"`
				Fields  map[string]string `json:"fields"`
			} `json:"integrations"`
		}
		if err := json.Unmarshal(data, &openpolyprintSettings); err == nil {
			for id, i := range openpolyprintSettings.Integrations {
				intgMgr.SetConfig(id, i.Fields)
			}
		}
	}

	var cfgMgr *config.ConfigManager
	if *dataDir != "" {
		cfgMgr = config.NewConfigManagerWithDir(*dataDir)
	} else {
		m, err := config.NewConfigManager()
		if err != nil {
			log.Fatalf("config manager: %v", err)
		}
		cfgMgr = m
	}

	cfg, err := cfgMgr.Load("default")
	if err != nil {
		log.Fatalf("load ankerctl default config: %v", err)
	}

	var mgr atomic.Pointer[printers.Manager]
	mgr.Store(buildManager(cfg))
	go func() {
		if m := mgr.Load(); m != nil {
			_ = m.ConnectAll(context.Background())
			m.Watchdog(context.Background())
		}
	}()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/api/logs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{"lines": logStore.Lines()})
		case http.MethodPost:
			logStore.Clear()
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/logs/download", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Content-Disposition", "attachment; filename=\"openpolyprint.log\"")
		_, _ = w.Write([]byte(logStore.String()))
	})
	mux.HandleFunc("/api/gcode", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			files, err := gcodeStore.List()
			if err != nil {
				http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
				return
			}
			_ = json.NewEncoder(w).Encode(files)
		case http.MethodPost:
			if err := r.ParseMultipartForm(50 << 20); err != nil {
				http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusBadRequest)
				return
			}
			h := r.MultipartForm.File["file"]
			if len(h) == 0 {
				http.Error(w, `{"error":"no file"}`, http.StatusBadRequest)
				return
			}
			f, err := h[0].Open()
			if err != nil {
				http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
				return
			}
			defer f.Close()
			printerID := ""
			if vals := r.MultipartForm.Value["printer_id"]; len(vals) > 0 {
				printerID = vals[0]
			}
			saved, err := gcodeStore.Save(h[0].Filename, printerID, f)
			if err != nil {
				http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
				return
			}
			_ = json.NewEncoder(w).Encode(saved)
		default:
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/gcode/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		switch r.Method {
		case http.MethodGet:
			data, err := gcodeStore.Load(id)
			if err != nil {
				http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "text/plain")
			w.Write(data)
		case http.MethodDelete:
			if err := gcodeStore.Delete(id); err != nil {
				http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		}
	})
	cameras.Mount(mux, cameraMgr)
	pi.Mount(mux, piMgr)
	mux.HandleFunc("/api/config", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			data, err := os.ReadFile(settingsFile)
			if err != nil {
				if os.IsNotExist(err) {
					http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
					return
				}
				http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
				return
			}
			_, _ = w.Write(data)
		case http.MethodPost:
			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, `{"error":"read body"}`, http.StatusBadRequest)
				return
			}
			if err := os.MkdirAll(settingsDir, 0o755); err != nil {
				http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
				return
			}
			if err := os.WriteFile(settingsFile, body, 0o600); err != nil {
				http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
				return
			}
			var openpolyprintSettings struct {
				Integrations map[string]struct {
					Enabled bool              `json:"enabled"`
					Fields  map[string]string `json:"fields"`
				} `json:"integrations"`
			}
			_ = json.Unmarshal(body, &openpolyprintSettings)
			for id, i := range openpolyprintSettings.Integrations {
				intgMgr.SetConfig(id, i.Fields)
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/anker/login", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"success":false,"message":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Email            string         `json:"email"`
			Password         string         `json:"password"`
			Region           string         `json:"region"`
			CaptchaID        string         `json:"captcha_id"`
			CaptchaAnswer    string         `json:"captcha_answer"`
			VerificationCode string         `json:"verification_code"`
			VerificationData map[string]any `json:"verification_data"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(anker.LoginResponse{Success: false, Message: "Invalid request body"})
			return
		}

		resp, newCfg, _ := anker.Login(req.Email, req.Password, req.Region, req.CaptchaID, req.CaptchaAnswer, req.VerificationCode, req.VerificationData, cfgMgr)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)

		if resp.Success && newCfg != nil {
			go func() {
				newMgr := buildManager(newCfg)
				oldMgr := mgr.Swap(newMgr)
				if oldMgr != nil {
					go func() { _ = oldMgr.DisconnectAll() }()
				}
				go func() { _ = newMgr.ConnectAll(context.Background()) }()
			}()
		}
	})
	mux.HandleFunc("/api/anker/import", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"success":false,"message":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		data, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(anker.LoginResponse{Success: false, Message: "Read body failed"})
			return
		}

		resp, newCfg, _ := anker.ImportLoginJSON(data, cfgMgr)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)

		if resp.Success && newCfg != nil {
			go func() {
				newMgr := buildManager(newCfg)
				oldMgr := mgr.Swap(newMgr)
				if oldMgr != nil {
					go func() { _ = oldMgr.DisconnectAll() }()
				}
				go func() { _ = newMgr.ConnectAll(context.Background()) }()
			}()
		}
	})
	mux.HandleFunc("/api/anker/detect", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, `{"success":false,"message":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		path := anker.FindLoginJSON()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"found": path != "",
			"path":  path,
		})
	})
	mux.HandleFunc("/api/anker/auto-import", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"success":false,"message":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		resp, newCfg, _ := anker.AutoImport(cfgMgr)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)

		if resp.Success && newCfg != nil {
			go func() {
				newMgr := buildManager(newCfg)
				oldMgr := mgr.Swap(newMgr)
				if oldMgr != nil {
					go func() { _ = oldMgr.DisconnectAll() }()
				}
				go func() { _ = newMgr.ConnectAll(context.Background()) }()
			}()
		}
	})
	mux.HandleFunc("/api/anker/config", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			cfg, _ := cfgMgr.Load("default")
			if cfg == nil {
				cfg = &config.Config{}
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(cfg)
		case http.MethodDelete:
			_ = cfgMgr.Delete("default")
			_ = os.Remove(filepath.Join(cfgMgr.ConfigDir(), "login.json"))
			oldMgr := mgr.Swap(buildManager(nil))
			if oldMgr != nil {
				go func() { _ = oldMgr.DisconnectAll() }()
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
		default:
			http.Error(w, `{"success":false,"message":"method not allowed"}`, http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/anker/export-keys", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, `{"success":false,"message":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		cfg, _ := cfgMgr.Load("default")
		if cfg == nil {
			http.Error(w, `{"error":"config not found"}`, http.StatusNotFound)
			return
		}
		type printerKeys struct {
			ID         string `json:"id"`
			Name       string `json:"name"`
			Model      string `json:"model"`
			SN         string `json:"sn"`
			WifiMAC    string `json:"wifi_mac"`
			IPAddr     string `json:"ip_addr"`
			P2PDUID    string `json:"p2p_duid"`
			P2PKey     string `json:"p2p_key"`
			MQTTKeyHex string `json:"mqtt_key_hex"`
		}
		printers := make([]printerKeys, 0, len(cfg.Printers))
		for _, p := range cfg.Printers {
			printers = append(printers, printerKeys{
				ID:         p.ID,
				Name:       p.Name,
				Model:      p.Model,
				SN:         p.SN,
				WifiMAC:    p.WifiMAC,
				IPAddr:     p.IPAddr,
				P2PDUID:    p.P2PDUID,
				P2PKey:     p.P2PKey,
				MQTTKeyHex: strings.ToUpper(fmt.Sprintf("%x", p.MQTTKey)),
			})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"printers": printers})
	})
	mux.HandleFunc("/api/anker/config/download", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, `{"success":false,"message":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		path := cfgMgr.ConfigPath("default")
		data, err := os.ReadFile(path)
		if err != nil {
			http.Error(w, `{"success":false,"message":"config not found"}`, http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", `attachment; filename="openpolyprint-anker-config.json"`)
		_, _ = w.Write(data)
	})
	mux.HandleFunc("/api/anker/login.json", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, `{"success":false,"message":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		path := filepath.Join(cfgMgr.ConfigDir(), "login.json")
		data, err := os.ReadFile(path)
		if err != nil {
			http.Error(w, `{"success":false,"message":"login.json not found"}`, http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", `attachment; filename="login.json"`)
		_, _ = w.Write(data)
	})
	mux.HandleFunc("/api/integrations", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(integrations.Registry)
	})
	mux.HandleFunc("/api/integrations/{id}/test", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		id := r.PathValue("id")
		var req struct {
			Message string            `json:"message"`
			Config  map[string]string `json:"config"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Message == "" {
			req.Message = "OpenPolyPrint integration test"
		}
		if err := intgMgr.Test(id, req.Message, req.Config); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
	})

	// ─── OctoPrint-compatible API ──────────────────────────────────────────
	// These endpoints allow slicers (PrusaSlicer, OrcaSlicer, Cura) to upload
	// G-code and start prints. Routing to a specific printer is done via:
	//   1. POST /api/files/{printerName}/local — explicit printer in path
	//   2. POST /api/files/local — uses the configured "slicer target" printer

	mux.HandleFunc("/api/version", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"api":    "0.1",
			"server": "2.0.0",
			"text":   "OpenPolyPrint OctoPrint-compatible API",
		})
	})

	mux.HandleFunc("/api/connection", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"current": map[string]string{"state": "Operational"},
			"options": map[string]any{
				"ports":           []string{"VIRTUAL"},
				"baudrates":       []int{0},
				"printerProfiles": []map[string]string{{"id": "default", "name": "OpenPolyPrint"}},
				"autoconnect":     false,
			},
		})
	})

	mux.HandleFunc("/api/printer", func(w http.ResponseWriter, r *http.Request) {
		target := loadSlicerTarget(settingsFile)
		var status printers.Status
		if target != "" {
			if d := mgr.Load().FindByName(target); d != nil {
				status, _ = d.Status()
			}
		}
		if status.ID == "" {
			statuses := mgr.Load().Statuses()
			if len(statuses) > 0 {
				status = statuses[0]
			}
		}
		stateText := "Offline"
		if status.Online {
			switch status.StatusText {
			case "Printing":
				stateText = "Printing"
			case "Paused":
				stateText = "Paused"
			default:
				stateText = "Operational"
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"state": map[string]string{"text": stateText},
			"temperature": map[string]any{
				"tool0": map[string]float64{"actual": status.Temps.Nozzle, "target": status.Temps.TargetNozzle, "offset": 0},
				"bed":   map[string]float64{"actual": status.Temps.Bed, "target": status.Temps.TargetBed, "offset": 0},
			},
		})
	})

	mux.HandleFunc("/api/job", func(w http.ResponseWriter, r *http.Request) {
		target := loadSlicerTarget(settingsFile)
		var status printers.Status
		if target != "" {
			if d := mgr.Load().FindByName(target); d != nil {
				status, _ = d.Status()
			}
		}
		if status.ID == "" {
			statuses := mgr.Load().Statuses()
			if len(statuses) > 0 {
				status = statuses[0]
			}
		}
		stateText := "Offline"
		if status.Online {
			switch status.StatusText {
			case "Printing":
				stateText = "Printing"
			case "Paused":
				stateText = "Paused"
			default:
				stateText = "Operational"
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"job": map[string]any{
				"file":               map[string]any{"name": status.CurrentFile, "origin": "local", "size": 0, "date": 0},
				"estimatedPrintTime": 0,
			},
			"progress": map[string]any{
				"completion":    float64(status.Progress),
				"filepos":       0,
				"printTime":     0,
				"printTimeLeft": 0,
			},
			"state": stateText,
		})
	})

	// OctoPrint file upload: POST /api/files/local or POST /api/files/{printer}/local
	mux.HandleFunc("/api/files/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		// Parse path: /api/files/{printer}/local or /api/files/local
		pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/files/"), "/")
		var printerName, action string
		if len(pathParts) >= 2 && pathParts[0] != "local" {
			printerName = pathParts[0]
			action = pathParts[1]
		} else if len(pathParts) >= 1 {
			action = pathParts[0]
		}

		// If action is "local", this is an upload. Otherwise it's a file select.
		if action == "local" {
			// File upload via multipart form
			if err := r.ParseMultipartForm(500 << 20); err != nil {
				http.Error(w, `{"error":"failed to parse multipart form"}`, http.StatusBadRequest)
				return
			}
			file, header, err := r.FormFile("file")
			if err != nil {
				http.Error(w, `{"error":"no file in form"}`, http.StatusBadRequest)
				return
			}
			defer file.Close()
			data, err := io.ReadAll(file)
			if err != nil {
				http.Error(w, `{"error":"failed to read file"}`, http.StatusBadRequest)
				return
			}

			// Resolve target printer
			m := mgr.Load()
			var driver printers.Driver
			if printerName != "" {
				driver = m.FindByName(printerName)
			}
			if driver == nil {
				target := loadSlicerTarget(settingsFile)
				if target != "" {
					driver = m.FindByName(target)
				}
			}
			if driver == nil {
				// Fall back to first available printer
				for _, d := range m.Drivers() {
					driver = d
					break
				}
			}
			if driver == nil {
				http.Error(w, `{"error":"no printer available"}`, http.StatusNotFound)
				return
			}

			filename := header.Filename
			if filename == "" {
				filename = "upload.gcode"
			}

			// Upload to printer
			if err := driver.UploadGCode(r.Context(), filename, data); err != nil {
				log.Printf("[slicer] upload %s to %s failed: %v", filename, driver.Name(), err)
				http.Error(w, fmt.Sprintf(`{"error":"upload failed: %s"}`, err.Error()), http.StatusInternalServerError)
				return
			}
			log.Printf("[slicer] uploaded %s (%d bytes) to %s", filename, len(data), driver.Name())

			// Check if the slicer requested to start printing immediately
			printNow := r.FormValue("print") == "true"
			if printNow {
				if err := driver.StartPrint(r.Context(), filename); err != nil {
					log.Printf("[slicer] start print %s on %s failed: %v", filename, driver.Name(), err)
				} else {
					log.Printf("[slicer] started print %s on %s", filename, driver.Name())
				}
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"files": map[string]any{
					"local": map[string]any{
						"name":   filename,
						"origin": "local",
						"size":   len(data),
						"type":   "machinecode",
					},
				},
				"done": true,
			})
			return
		}

		// File select: POST /api/files/{printer}/{filename} with {"command":"select","print":true}
		// or POST /api/files/{filename} with {"command":"select","print":true}
		var req struct {
			Command string `json:"command"`
			Print   bool   `json:"print"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)

		if req.Command != "select" {
			http.Error(w, `{"error":"unsupported command"}`, http.StatusBadRequest)
			return
		}

		// The filename is the last path segment
		fileToPrint := pathParts[len(pathParts)-1]
		if printerName == "" {
			// If no printer in path, the first segment is the filename
			fileToPrint = pathParts[0]
		}

		m := mgr.Load()
		var driver printers.Driver
		if printerName != "" {
			driver = m.FindByName(printerName)
		}
		if driver == nil {
			target := loadSlicerTarget(settingsFile)
			if target != "" {
				driver = m.FindByName(target)
			}
		}
		if driver == nil {
			for _, d := range m.Drivers() {
				driver = d
				break
			}
		}
		if driver == nil {
			http.Error(w, `{"error":"no printer available"}`, http.StatusNotFound)
			return
		}

		if req.Print {
			if err := driver.StartPrint(r.Context(), fileToPrint); err != nil {
				log.Printf("[slicer] start print %s on %s failed: %v", fileToPrint, driver.Name(), err)
				http.Error(w, fmt.Sprintf(`{"error":"start print failed: %s"}`, err.Error()), http.StatusInternalServerError)
				return
			}
			log.Printf("[slicer] started print %s on %s", fileToPrint, driver.Name())
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{})
	})

	mux.HandleFunc("/api/printers", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(mgr.Load().Statuses())
		case http.MethodPost:
			var p printers.PrinterConfig
			if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
				http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
				return
			}
			if p.Name == "" || p.Type == "" {
				http.Error(w, `{"error":"name and type required"}`, http.StatusBadRequest)
				return
			}
			if p.ID == "" {
				p.ID = fmt.Sprintf("printer_%d", time.Now().UnixNano())
			}
			manualPrinters = append(manualPrinters, p)
			if err := saveManualPrinters(filepath.Join(settingsDir, "printers.json"), manualPrinters); err != nil {
				log.Printf("save printers: %v", err)
				http.Error(w, `{"error":"failed to save printer"}`, http.StatusInternalServerError)
				return
			}
			mgr.Store(buildManager(cfg))
			go func() {
				m := mgr.Load()
				if m != nil {
					_ = m.ConnectAll(context.Background())
					m.Watchdog(context.Background())
				}
			}()
			_ = json.NewEncoder(w).Encode(mgr.Load().Statuses())
		default:
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/printers/{id}", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		id := r.PathValue("id")
		found := false
		for i, p := range manualPrinters {
			if p.ID == id {
				manualPrinters = append(manualPrinters[:i], manualPrinters[i+1:]...)
				found = true
				break
			}
		}
		if !found {
			http.Error(w, `{"error":"printer not found"}`, http.StatusNotFound)
			return
		}
		if err := saveManualPrinters(filepath.Join(settingsDir, "printers.json"), manualPrinters); err != nil {
			log.Printf("save printers: %v", err)
			http.Error(w, `{"error":"failed to save printers"}`, http.StatusInternalServerError)
			return
		}
		mgr.Store(buildManager(cfg))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
	})
	mux.HandleFunc("/api/printers/{id}/status", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		d := mgr.Load().Find(id)
		if d == nil {
			http.Error(w, `{"error":"printer not found"}`, http.StatusNotFound)
			return
		}
		s, err := d.Status()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(s)
	})
	mux.HandleFunc("/api/printers/{id}/pause", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		id := r.PathValue("id")
		if err := mgr.Load().PausePrint(r.Context(), id); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
	})
	mux.HandleFunc("/api/printers/{id}/stop", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		id := r.PathValue("id")
		if err := mgr.Load().StopPrint(r.Context(), id); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
	})
	mux.HandleFunc("/api/printers/{id}/home", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		id := r.PathValue("id")
		if err := mgr.Load().Home(r.Context(), id); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
	})
	mux.HandleFunc("/api/printers/{id}/preheat", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		id := r.PathValue("id")
		var req struct {
			Nozzle float64 `json:"nozzle"`
			Bed    float64 `json:"bed"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
			return
		}
		if req.Nozzle == 0 && req.Bed == 0 {
			req.Nozzle = 200
			req.Bed = 60
		}
		if err := mgr.Load().Preheat(r.Context(), id, req.Nozzle, req.Bed); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
	})
	mux.HandleFunc("/api/printers/{id}/cooldown", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		id := r.PathValue("id")
		if err := mgr.Load().Cooldown(r.Context(), id); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
	})
	mux.HandleFunc("/api/printers/{id}/level", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		id := r.PathValue("id")
		if err := mgr.Load().AutoLevel(r.Context(), id); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
	})
	mux.HandleFunc("/api/printers/{id}/gcode", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		id := r.PathValue("id")
		var req struct {
			Command string `json:"command"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
			return
		}
		if err := mgr.Load().SendGCode(r.Context(), id, req.Command); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
	})
	mux.HandleFunc("/api/history", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"history": historyStore.List()})
		case http.MethodPost:
			var req struct {
				Printer   string    `json:"printer"`
				File      string    `json:"file"`
				Result    string    `json:"result"`
				StartedAt time.Time `json:"startedAt"`
				EndedAt   time.Time `json:"endedAt"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
				return
			}
			if req.Printer == "" || req.File == "" || req.EndedAt.IsZero() {
				http.Error(w, `{"error":"missing fields"}`, http.StatusBadRequest)
				return
			}
			rec := historyStore.Add(req.Printer, req.File, req.Result, req.StartedAt, req.EndedAt)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(rec)
		case http.MethodDelete:
			if err := historyStore.Clear(); err != nil {
				http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/history/{id}", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		if err := historyStore.Delete(r.PathValue("id")); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	go trackHistory(context.Background(), &mgr, cameraMgr, historyStore, settingsFile, intgMgr)

	// Serve the built frontend if dist/ exists next to the binary.
	dist := findDistDir()
	if dist != "" {
		fs := http.FileServer(http.Dir(dist))
		// /recordings is a React route, but /recordings/ is the static video file server;
		// serve the SPA for the exact route so deep refresh works.
		mux.HandleFunc("/recordings", func(w http.ResponseWriter, r *http.Request) {
			r.URL.Path = "/"
			fs.ServeHTTP(w, r)
		})
		// Serve manifest.json with the correct MIME type for PWA install prompts.
		mux.HandleFunc("/manifest.json", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/manifest+json")
			w.Header().Set("Cache-Control", "no-cache")
			http.ServeFile(w, r, filepath.Join(dist, "manifest.json"))
		})
		// Serve service worker with correct scope and no cache.
		mux.HandleFunc("/sw.js", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/javascript")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Service-Worker-Allowed", "/")
			http.ServeFile(w, r, filepath.Join(dist, "sw.js"))
		})
		mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Set correct MIME types for common static files
			ext := filepath.Ext(r.URL.Path)
			switch ext {
			case ".json":
				w.Header().Set("Content-Type", "application/json")
			case ".js":
				w.Header().Set("Content-Type", "application/javascript")
			case ".css":
				w.Header().Set("Content-Type", "text/css")
			case ".svg":
				w.Header().Set("Content-Type", "image/svg+xml")
			case ".png":
				w.Header().Set("Content-Type", "image/png")
			case ".webmanifest":
				w.Header().Set("Content-Type", "application/manifest+json")
			}
			if _, err := os.Stat(filepath.Join(dist, r.URL.Path)); os.IsNotExist(err) {
				r.URL.Path = "/"
			}
			fs.ServeHTTP(w, r)
		}))
	} else {
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("OpenPolyPrint API is running. Frontend dist/ not found.\n"))
		})
	}

	server := &http.Server{Addr: *addr, Handler: mux}
	go func() {
		fmt.Printf("OpenPolyPrint listening on http://localhost%s\n", *addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http server: %v", err)
		}
	}()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	<-c

	fmt.Println("\nShutting down...")
	_ = mgr.Load().DisconnectAll()
	_ = server.Close()
}
