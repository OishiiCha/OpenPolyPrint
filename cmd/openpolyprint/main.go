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

func buildManager(cfg *config.Config) *printers.Manager {
	var drivers []printers.Driver
	if cfg != nil {
		for _, p := range cfg.Printers {
			drivers = append(drivers, anker.NewDriver(p, cfg.Account))
		}
	}
	return printers.NewManager(drivers)
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
func trackHistory(ctx context.Context, mgr *atomic.Pointer[printers.Manager], cameraMgr *cameras.Manager, store *history.Store, settingsFile string) {
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
						if s.CurrentFile == "" {
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

	userConfigDir, err := os.UserConfigDir()
	if err != nil {
		log.Fatalf("user config dir: %v", err)
	}
	settingsDir := filepath.Join(userConfigDir, "openpolyprint")
	settingsFile := filepath.Join(settingsDir, "settings.json")
	gcodeDir := filepath.Join(settingsDir, "gcode")

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
	mux.HandleFunc("/api/printers", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(mgr.Load().Statuses())
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

	go trackHistory(context.Background(), &mgr, cameraMgr, historyStore, settingsFile)

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
		mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
