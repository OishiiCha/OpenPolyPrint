package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lucas/openpolyprint/internal/ai"
	"github.com/lucas/openpolyprint/internal/analytics"
	"github.com/lucas/openpolyprint/internal/anker"
	"github.com/lucas/openpolyprint/internal/anker/proto/config"
	"github.com/lucas/openpolyprint/internal/auth"
	"github.com/lucas/openpolyprint/internal/cameras"
	"github.com/lucas/openpolyprint/internal/envconfig"
	"github.com/lucas/openpolyprint/internal/filament"
	"github.com/lucas/openpolyprint/internal/gcode"
	"github.com/lucas/openpolyprint/internal/history"
	"github.com/lucas/openpolyprint/internal/integrations"
	"github.com/lucas/openpolyprint/internal/klipper"
	"github.com/lucas/openpolyprint/internal/logstore"
	"github.com/lucas/openpolyprint/internal/maintenance"
	"github.com/lucas/openpolyprint/internal/pi"
	"github.com/lucas/openpolyprint/internal/printers"
	"github.com/lucas/openpolyprint/internal/printsession"
	"github.com/lucas/openpolyprint/internal/profileconverter"
	"github.com/lucas/openpolyprint/internal/profilefiles"
	"github.com/lucas/openpolyprint/internal/profiles"
	"github.com/lucas/openpolyprint/internal/push"
	"github.com/lucas/openpolyprint/internal/queue"
	"github.com/lucas/openpolyprint/internal/smartplug"
	"github.com/lucas/openpolyprint/internal/stlfiles"
	"github.com/lucas/openpolyprint/internal/tempstore"
	"github.com/lucas/openpolyprint/internal/tlsautocert"
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

// printerAliases maps printer ID → custom display name.
// Loaded from printer_aliases.json in the settings directory.
var printerAliases map[string]string
var printerAliasesMu sync.RWMutex

func loadPrinterAliases(path string) map[string]string {
	data, err := os.ReadFile(path)
	if err != nil {
		return map[string]string{}
	}
	var out map[string]string
	if err := json.Unmarshal(data, &out); err != nil {
		log.Printf("printer aliases load: %v", err)
		return map[string]string{}
	}
	if out == nil {
		out = map[string]string{}
	}
	return out
}

func savePrinterAliases(path string, m map[string]string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}

// applyAliases replaces printer names with custom aliases if set.
func applyAliases(statuses []printers.Status) []printers.Status {
	printerAliasesMu.RLock()
	defer printerAliasesMu.RUnlock()
	for i := range statuses {
		if alias, ok := printerAliases[statuses[i].ID]; ok && alias != "" {
			statuses[i].Name = alias
		}
	}
	return statuses
}

// findPrinterByNameOrAlias finds a printer driver by its original name or alias.
func findPrinterByNameOrAlias(m *printers.Manager, name string) printers.Driver {
	if d := m.FindByName(name); d != nil {
		return d
	}
	// Check if name matches an alias
	printerAliasesMu.RLock()
	defer printerAliasesMu.RUnlock()
	for id, alias := range printerAliases {
		if strings.EqualFold(alias, name) {
			if d := m.Find(id); d != nil {
				return d
			}
		}
	}
	return nil
}

func buildManager(cfg *config.Config) *printers.Manager {
	var drivers []printers.Driver
	if cfg != nil {
		for _, p := range cfg.Printers {
			// Anker config.Printer doesn't have a Type field; all printers
			// from the Anker config are Anker printers.
			drivers = append(drivers, anker.NewDriver(p, cfg.Account))
		}
	}
	for _, p := range manualPrinters {
		switch p.Type {
		case "klipper":
			drivers = append(drivers, klipper.NewDriver(p))
		default:
			drivers = append(drivers, printers.NewStaticDriver(p))
		}
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

func safeName(s string) string {
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.ReplaceAll(s, "\\", "_")
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.TrimSuffix(s, ".gcode")
	s = strings.TrimSuffix(s, ".mkv")
	s = strings.TrimSuffix(s, ".avi")
	return s
}

func absInt64(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
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
func trackHistory(ctx context.Context, mgr *atomic.Pointer[printers.Manager], cameraMgr *cameras.Manager, store *history.Store, settingsFile string, intgMgr *integrations.Manager, tempStore *tempstore.Store, queueStore *queue.Store, plugMgr *smartplug.Manager, pushMgr *push.Manager, sessMgr *printsession.Manager) {
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
				tempStore.Record(s.ID, s.Temps.Nozzle, s.Temps.TargetNozzle, s.Temps.Bed, s.Temps.TargetBed)
				prev, hasPrev := last[s.ID]
				if s.StatusText == "Printing" {
					if _, ok := started[s.ID]; !ok {
						started[s.ID] = time.Now()
					}
					// Auto-start print session data collection for AI
					if !sessMgr.IsActive(s.ID) {
						sessMgr.Start(s.ID, s.Name, s.CurrentFile)
					}
					sessMgr.RecordTemp(s.ID, s.Temps.Nozzle, s.Temps.TargetNozzle, s.Temps.Bed, s.Temps.TargetBed, float64(s.Progress))
					sessMgr.RecordStatus(s.ID, s.StatusText, float64(s.Progress), s.CurrentFile)

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
						// Paused or transient â€” donâ€™t record yet
						last[s.ID] = s
						continue
					}
					store.Add(s.Name, file, result, start, time.Now())
					delete(started, s.ID)

					// Stop print session data collection
					sessMgr.Stop(s.ID, result)

					// Send push notification
					switch result {
					case "Success":
						pushMgr.Send("Print finished", s.Name+": "+file+" completed successfully")
					case "Failed":
						pushMgr.Send("Print failed", s.Name+": "+file+" failed")
					case "Cancelled":
						pushMgr.Send("Print cancelled", s.Name+": "+file+" was cancelled")
					}

					// Auto-off smart plugs for this printer
					if result == "Success" {
						plugMgr.AutoOffForPrinter(s.ID)
					}

					// Auto-start next queued item for this printer
					if result == "Success" || result == "Cancelled" {
						if next := queueStore.NextPending(s.ID); next != nil {
							queueStore.UpdateStatus(next.ID, "printing", "")
							if d := m.Find(s.ID); d != nil {
								if err := d.StartPrint(ctx, next.Filename); err != nil {
									log.Printf("[queue] auto-start %s on %s failed: %v", next.Filename, s.Name, err)
									queueStore.UpdateStatus(next.ID, "failed", err.Error())
								} else {
									log.Printf("[queue] auto-started %s on %s", next.Filename, s.Name)
								}
							}
						}
					}
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

// envSecretConfig returns config values sourced from environment variables
// (or .env file). These take precedence over settings.json values.
func envSecretConfig() map[string]any {
	m := map[string]any{
		"geminiApiKey":   envconfig.Get("GEMINI_API_KEY", ""),
		"envAnkerEmail":  envconfig.Get("ANKER_EMAIL", ""),
		"envAnkerRegion": envconfig.Get("ANKER_REGION", ""),
		"authEnabled":    envconfig.Get("AUTH_PASSCODE", "") != "",
	}
	// Only include geminiEnabled if the env var is explicitly set —
	// otherwise the default (false) would override the saved value in
	// settings.json every time the config is loaded.
	if v, ok := os.LookupEnv("GEMINI_ENABLED"); ok {
		switch strings.ToLower(v) {
		case "true", "1", "yes", "on":
			m["geminiEnabled"] = true
		case "false", "0", "no", "off":
			m["geminiEnabled"] = false
		}
	}
	return m
}

// loadAuthPasscode resolves the auth passcode from env, then settings.json.
func loadAuthPasscode(settingsFile string) string {
	if pc := envconfig.Get("AUTH_PASSCODE", ""); pc != "" {
		return pc
	}
	if data, err := os.ReadFile(settingsFile); err == nil {
		var cfg map[string]any
		if json.Unmarshal(data, &cfg) == nil {
			if pc, ok := cfg["authPasscode"].(string); ok {
				return pc
			}
		}
	}
	return ""
}

// resolveAPIKey resolves the Gemini API key from settings.json, then env.
func resolveAPIKey(settingsFile string) string {
	if data, err := os.ReadFile(settingsFile); err == nil {
		var cfg map[string]any
		if json.Unmarshal(data, &cfg) == nil {
			if v, ok := cfg["geminiApiKey"].(string); ok && v != "" {
				return v
			}
		}
	}
	return envconfig.Get("GEMINI_API_KEY", "")
}

func main() {
	var (
		dataDir   = flag.String("data-dir", "", "directory that holds ankerctl default.json (default: platform config dir/ankerctl)")
		addr      = flag.String("addr", ":443", "HTTPS listen address (HTTP always listens on :80 when TLS is enabled; when TLS is disabled this is the HTTP port)")
		enableTLS = flag.Bool("tls", true, "enable HTTPS with auto-generated self-signed certificate")
	)
	flag.Parse()

	logStore := logstore.New(2000)
	log.SetOutput(io.MultiWriter(os.Stderr, logStore))

	// Load .env file if present (from CWD, settings dir, or /data).
	// Existing environment variables take precedence over .env values.
	for _, dir := range []string{".", filepath.Dir(os.Args[0])} {
		if err := envconfig.LoadDir(dir); err != nil {
			log.Printf("[env] warning: could not load .env from %s: %v", dir, err)
		}
	}

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
	printerAliases = loadPrinterAliases(filepath.Join(settingsDir, "printer_aliases.json"))

	gcodeStore, err := gcode.NewStore(gcodeDir)
	if err != nil {
		log.Fatalf("gcode store: %v", err)
	}

	historyStore := history.NewStore(settingsDir)
	tempStore := tempstore.New(600)
	queueStore := queue.NewStore(settingsDir)
	filamentStore := filament.NewStore(settingsDir)
	profileStore := profiles.NewStore(settingsDir)
	profileFilesDir := filepath.Join(settingsDir, "profilefiles")
	profileFilesStore, err := profilefiles.NewStore(profileFilesDir)
	if err != nil {
		log.Printf("profile files store: %v", err)
	}
	stlFilesDir := filepath.Join(settingsDir, "stlfiles")
	stlFilesStore, err := stlfiles.NewStore(stlFilesDir)
	if err != nil {
		log.Printf("stl files store: %v", err)
	}
	maintStore := maintenance.NewStore(settingsDir)
	plugMgr := smartplug.NewManager()
	pushMgr := push.NewManager(settingsDir)

	cameraMgr := cameras.NewManager(settingsDir)
	// Start all enabled camera streams at startup so frames are immediately
	// available when a browser connects (no black screen / 1.5s wait).
	cameraMgr.Streamers().StartAllFromSettings(cameraMgr.Store())
	// Watchdog: auto-restart streams that crash or go stale.
	cameraMgr.Streamers().StartWatchdog(cameraMgr.Store())
	piMgr := pi.NewManagerGroup(settingsDir)
	sessMgr := printsession.NewManager(filepath.Join(settingsDir, "..", "recordings", "sessions"))
	promptStore := ai.NewPromptStore(filepath.Join(settingsDir, "ai_prompts.json"))
	chatStore := ai.NewChatStore(filepath.Join(settingsDir, "ai_chat"))

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

	// Apply env-based integration config (env takes precedence)
	envIntegrations := map[string]map[string]string{
		"telegram": {
			"token":   envconfig.Get("TELEGRAM_BOT_TOKEN", ""),
			"chat_id": envconfig.Get("TELEGRAM_CHAT_ID", ""),
		},
		"discord": {
			"webhook_url": envconfig.Get("DISCORD_WEBHOOK_URL", ""),
		},
		"n8n": {
			"webhook_url": envconfig.Get("N8N_WEBHOOK_URL", ""),
		},
		"obico": {
			"obico_token": envconfig.Get("OBICO_TOKEN", ""),
		},
	}
	for id, fields := range envIntegrations {
		// Only set if at least one field is non-empty
		hasValue := false
		for _, v := range fields {
			if v != "" {
				hasValue = true
				break
			}
		}
		if hasValue {
			intgMgr.SetConfig(id, fields)
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

	// Auto-login to Anker if credentials are provided via env vars
	if email := envconfig.Get("ANKER_EMAIL", ""); email != "" {
		if password := envconfig.Get("ANKER_PASSWORD", ""); password != "" {
			region := envconfig.Get("ANKER_REGION", "NA")
			go func() {
				log.Printf("[anker] auto-login from env: %s (region %s)", email, region)
				resp, newCfg, _ := anker.Login(email, password, region, "", "", "", nil, cfgMgr)
				if resp.Success && newCfg != nil {
					newMgr := buildManager(newCfg)
					oldMgr := mgr.Swap(newMgr)
					if oldMgr != nil {
						_ = oldMgr.DisconnectAll()
					}
					_ = newMgr.ConnectAll(context.Background())
					go newMgr.Watchdog(context.Background())
					log.Printf("[anker] auto-login successful: %d printer(s)", len(newCfg.Printers))
				} else if resp.Message != "" {
					log.Printf("[anker] auto-login failed: %s", resp.Message)
				}
			}()
		}
	}

	mux := http.NewServeMux()
	authMgr := auth.NewManager(loadAuthPasscode(settingsFile))

	// Auth endpoints
	mux.HandleFunc("/api/auth/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"enabled": authMgr.Enabled(),
		})
	})
	mux.HandleFunc("/api/auth/login", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Passcode string `json:"passcode"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
			return
		}
		token, ok := authMgr.Login(req.Passcode)
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"invalid passcode"}`))
			return
		}
		// Set cookie
		http.SetCookie(w, &http.Cookie{
			Name:     "openpolyprint_session",
			Value:    token,
			Path:     "/",
			MaxAge:   7 * 24 * 3600,
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		})
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "token": token})
	})
	mux.HandleFunc("/api/auth/logout", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		// Extract token from cookie or header
		if cookie, err := r.Cookie("openpolyprint_session"); err == nil {
			authMgr.Logout(cookie.Value)
		}
		// Clear cookie
		http.SetCookie(w, &http.Cookie{
			Name:     "openpolyprint_session",
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		})
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
	})

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

	// G-code timeline — returns timestamped segments for visualization sync
	mux.HandleFunc("/api/gcode/{id}/timeline", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		segments, err := gcodeStore.Timeline(id)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(segments)
	})

	// G-code thumbnail — returns embedded thumbnail as data URI
	mux.HandleFunc("/api/gcode/{id}/thumbnail", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		path := gcodeStore.FilePath(id)
		_, dataURI := gcode.ExtractThumbnail(path)
		if dataURI == "" {
			http.Error(w, `{"error":"no thumbnail"}`, http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"thumbnail": dataURI})
	})

	// Timelapse frames — list frame directories and individual frames
	mux.HandleFunc("/api/timelapse-frames", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		dirs, err := cameras.ListFrameDirs()
		if err != nil {
			http.Error(w, `{"error":"failed to list frames"}`, http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(dirs)
	})

	mux.HandleFunc("/api/timelapse-frames/{dir}", func(w http.ResponseWriter, r *http.Request) {
		dir := r.PathValue("dir")
		frames, err := cameras.ListFrames(dir)
		if err != nil {
			http.Error(w, `{"error":"failed to list frames"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(frames)
	})

	mux.HandleFunc("/api/timelapse-frames/{dir}/meta", func(w http.ResponseWriter, r *http.Request) {
		dir := r.PathValue("dir")
		metas, err := cameras.ListFrameMeta(dir)
		if err != nil {
			http.Error(w, `{"error":"failed to list frame metadata"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(metas)
	})

	// AI analysis — analyze a timelapse frame with G-code + temp context using Gemini
	mux.HandleFunc("/api/ai/analyze", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			APIKey         string  `json:"apiKey"`
			FrameDir       string  `json:"frameDir"`
			FrameNum       int     `json:"frameNum"`
			ElapsedSec     float64 `json:"elapsedSec"`
			IntervalSec    float64 `json:"intervalSec"`
			GCodeID        string  `json:"gcodeId"`
			PrinterName    string  `json:"printerName"`
			Filename       string  `json:"filename"`
			SessionID      string  `json:"sessionId"`
			PromptOverride string  `json:"promptOverride"`
			CustomPrompt   string  `json:"customPrompt"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
			return
		}

		// Resolve API key: request > settings.json > env
		apiKey := req.APIKey
		if apiKey == "" {
			if data, err := os.ReadFile(settingsFile); err == nil {
				var cfg map[string]any
				_ = json.Unmarshal(data, &cfg)
				if v, ok := cfg["geminiApiKey"].(string); ok {
					apiKey = v
				}
			}
		}
		if apiKey == "" {
			apiKey = envconfig.Get("GEMINI_API_KEY", "")
		}
		if apiKey == "" {
			http.Error(w, `{"error":"no Gemini API key configured. Set one in Settings or via GEMINI_API_KEY env var."}`, http.StatusBadRequest)
			return
		}

		// Find the frame file
		framesDir := filepath.Join("recordings", "timelapse", req.FrameDir)
		var framePath string
		var frameNum int
		if req.FrameNum > 0 {
			framePath = filepath.Join(framesDir, fmt.Sprintf("frame_%06d.jpg", req.FrameNum))
			frameNum = req.FrameNum
		} else {
			fp, fn, err := ai.FindFrameForTime(framesDir, req.ElapsedSec, req.IntervalSec)
			if err != nil {
				http.Error(w, `{"error":"frame not found: `+err.Error()+`"}`, http.StatusNotFound)
				return
			}
			framePath = fp
			frameNum = fn
		}

		// Get G-code timeline segment at this time
		var gcodeSnippet string
		var layer int
		var x, y, z float64
		if req.GCodeID != "" {
			segments, err := gcodeStore.Timeline(req.GCodeID)
			if err == nil && len(segments) > 0 {
				seg := gcode.SegmentAtTime(segments, req.ElapsedSec)
				if seg != nil {
					layer = seg.Layer
					x, y, z = seg.X, seg.Y, seg.Z
					// Build snippet: 5 lines before and after
					startLine := seg.LineNum - 5
					if startLine < 1 {
						startLine = 1
					}
					endLine := seg.LineNum + 5
					data, _ := gcodeStore.Load(req.GCodeID)
					lines := strings.Split(string(data), "\n")
					if endLine > len(lines) {
						endLine = len(lines)
					}
					if startLine <= len(lines) {
						gcodeSnippet = strings.Join(lines[startLine-1:endLine], "\n")
					}
				}
			}
		}

		// Get temperature at this time
		// If a session ID is provided, use the session's temp samples (accurate
		// temp at the given elapsed time). Otherwise fall back to tempstore.
		var nozzleTemp, targetNozzle, bedTemp, targetBed float64
		if req.SessionID != "" {
			if sess, err := sessMgr.GetSavedSession(req.SessionID); err == nil && len(sess.TempSamples) > 0 {
				// Find the temp sample closest to the elapsed time
				// Temp samples have unix timestamps; convert elapsed to absolute time
				targetTime := sess.StartTime.Add(time.Duration(req.ElapsedSec * float64(time.Second)))
				var bestSample *printsession.TempSample
				var bestDiff time.Duration = 1 << 62
				for i := range sess.TempSamples {
					diff := time.Duration(absInt64(sess.TempSamples[i].Time - targetTime.Unix()))
					if diff < bestDiff {
						bestDiff = diff
						bestSample = &sess.TempSamples[i]
					}
				}
				if bestSample != nil {
					nozzleTemp = bestSample.Nozzle
					targetNozzle = bestSample.TargetNozzle
					bedTemp = bestSample.Bed
					targetBed = bestSample.TargetBed
				}
			}
		}
		if nozzleTemp == 0 && targetNozzle == 0 {
			// Fall back to tempstore latest
			allTemps := tempStore.GetAll()
			for _, samples := range allTemps {
				if len(samples) > 0 {
					last := samples[len(samples)-1]
					nozzleTemp = last.Nozzle
					targetNozzle = last.TargetNozzle
					bedTemp = last.Bed
					targetBed = last.TargetBed
				}
				break
			}
		}

		analysisReq := ai.AnalysisRequest{
			APIKey:         apiKey,
			FramePath:      framePath,
			FrameDir:       req.FrameDir,
			FrameNum:       frameNum,
			ElapsedSec:     req.ElapsedSec,
			GCodeLine:      0,
			GCodeSnippet:   gcodeSnippet,
			Layer:          layer,
			X:              x,
			Y:              y,
			Z:              z,
			NozzleTemp:     nozzleTemp,
			TargetNozzle:   targetNozzle,
			BedTemp:        bedTemp,
			TargetBed:      targetBed,
			PrinterName:    req.PrinterName,
			Filename:       req.Filename,
			PromptOverride: req.PromptOverride,
			CustomPrompt:   req.CustomPrompt,
		}

		result, err := ai.Analyze(analysisReq)
		if err != nil {
			log.Printf("[ai] analysis failed: %v", err)
			http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(result)
	})

	// AI prompt — generate the default prompt based on session/frame context
	mux.HandleFunc("/api/ai/prompt", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			FrameDir    string  `json:"frameDir"`
			FrameNum    int     `json:"frameNum"`
			ElapsedSec  float64 `json:"elapsedSec"`
			IntervalSec float64 `json:"intervalSec"`
			GCodeID     string  `json:"gcodeId"`
			PrinterName string  `json:"printerName"`
			Filename    string  `json:"filename"`
			SessionID   string  `json:"sessionId"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
			return
		}

		// Build a mock AnalysisRequest with whatever data is available
		// to generate the default prompt
		mockReq := ai.AnalysisRequest{
			FrameNum:    req.FrameNum,
			ElapsedSec:  req.ElapsedSec,
			PrinterName: req.PrinterName,
			Filename:    req.Filename,
		}

		// Get G-code context if available
		if req.GCodeID != "" {
			segments, err := gcodeStore.Timeline(req.GCodeID)
			if err == nil && len(segments) > 0 {
				seg := gcode.SegmentAtTime(segments, req.ElapsedSec)
				if seg != nil {
					mockReq.Layer = seg.Layer
					mockReq.X, mockReq.Y, mockReq.Z = seg.X, seg.Y, seg.Z
					startLine := seg.LineNum - 5
					if startLine < 1 {
						startLine = 1
					}
					endLine := seg.LineNum + 5
					data, _ := gcodeStore.Load(req.GCodeID)
					lines := strings.Split(string(data), "\n")
					if endLine > len(lines) {
						endLine = len(lines)
					}
					if startLine <= len(lines) {
						mockReq.GCodeSnippet = strings.Join(lines[startLine-1:endLine], "\n")
					}
				}
			}
		}

		// Get temp data from session if available
		if req.SessionID != "" {
			if sess, err := sessMgr.GetSavedSession(req.SessionID); err == nil && len(sess.TempSamples) > 0 {
				targetTime := sess.StartTime.Add(time.Duration(req.ElapsedSec * float64(time.Second)))
				var bestSample *printsession.TempSample
				var bestDiff int64 = 1 << 60
				for i := range sess.TempSamples {
					diff := absInt64(sess.TempSamples[i].Time - targetTime.Unix())
					if diff < bestDiff {
						bestDiff = diff
						bestSample = &sess.TempSamples[i]
					}
				}
				if bestSample != nil {
					mockReq.NozzleTemp = bestSample.Nozzle
					mockReq.TargetNozzle = bestSample.TargetNozzle
					mockReq.BedTemp = bestSample.Bed
					mockReq.TargetBed = bestSample.TargetBed
				}
			}
		}

		prompt := ai.BuildDefaultPrompt(mockReq)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"prompt": prompt})
	})

	// AI prompt presets — CRUD
	mux.HandleFunc("/api/ai/prompts", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			presets, err := promptStore.List()
			if err != nil {
				http.Error(w, `{"error":"failed to list prompts"}`, http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"prompts": presets})
		case http.MethodPost:
			var req struct {
				ID     string `json:"id"`
				Name   string `json:"name"`
				Prompt string `json:"prompt"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
				return
			}
			if req.Name == "" {
				http.Error(w, `{"error":"name required"}`, http.StatusBadRequest)
				return
			}
			preset, err := promptStore.Save(req.ID, req.Name, req.Prompt)
			if err != nil {
				http.Error(w, `{"error":"failed to save prompt"}`, http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(preset)
		default:
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/ai/prompts/{id}", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		id := r.PathValue("id")
		if err := promptStore.Delete(id); err != nil {
			http.Error(w, `{"error":"prompt not found"}`, http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
	})

	// ── AI Chat: live printer analysis with camera frames ─────────────
	// Capture a single camera frame as JPEG for AI analysis (multi-snapshot).
	mux.HandleFunc("/api/ai/snapshot/{printerId}", func(w http.ResponseWriter, r *http.Request) {
		printerID := r.PathValue("printerId")
		for _, cam := range cameraMgr.Store().GetCameras() {
			if !cam.Enabled || cam.PrinterID != printerID {
				continue
			}
			if cam.Type != "usb" && cam.Type != "rpicam" {
				continue
			}
			frameData := cameraMgr.Streamers().GetFrameForCamera(cam.ID)
			if frameData == nil {
				continue
			}
			w.Header().Set("Content-Type", "image/jpeg")
			w.Header().Set("Cache-Control", "no-cache")
			w.Write(frameData)
			return
		}
		http.Error(w, `{"error":"no camera frame available"}`, http.StatusNotFound)
	})

	// Analyze with pre-captured image(s) — creates a chat conversation,
	// saves the image(s), sends to Gemini, and returns the full conversation.
	// Used by the dashboard "Ask AI" multi-snapshot feature.
	mux.HandleFunc("/api/ai/analyze", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			PrinterID   string   `json:"printerId"`
			PrinterName string   `json:"printerName"`
			Images      []string `json:"images"`     // base64-encoded JPEG images
			Message     string   `json:"message"`    // user's prompt
			Context     string   `json:"context"`    // extra context text (printer data, file content, etc.)
			SourceType  string   `json:"sourceType"` // "printer", "profile", "stl"
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
			return
		}
		apiKey := resolveAPIKey(settingsFile)
		if apiKey == "" {
			http.Error(w, `{"error":"no Gemini API key configured"}`, http.StatusBadRequest)
			return
		}

		// Create a new conversation
		conv := chatStore.Create(req.PrinterID, req.PrinterName, "")

		// Save images and build message
		var imagePaths []string
		for _, b64 := range req.Images {
			imgData, err := base64.StdEncoding.DecodeString(b64)
			if err != nil {
				continue
			}
			relPath, err := chatStore.SaveImage(conv.ID, imgData, ".jpg")
			if err != nil {
				continue
			}
			imagePaths = append(imagePaths, relPath)
		}

		// Build the system prompt based on source type
		systemPrompt := "You are a 3D printing expert assistant. "
		switch req.SourceType {
		case "printer":
			systemPrompt += "The user is sharing live camera snapshots from their 3D printer along with printer status data. Analyze the images and data to identify any issues, suggest improvements, and provide helpful advice about the print quality and printer status."
		case "profile":
			systemPrompt += "The user is sharing a slicer profile file for analysis. Review the settings and provide recommendations for optimization, print quality improvements, and any potential issues."
		case "stl":
			systemPrompt += "The user is sharing a screenshot of a 3D model (STL file). Analyze the model geometry, orientation, and suggest optimal print settings, supports, and any potential printing challenges."
		default:
			systemPrompt += "Analyze the provided information and help the user with their 3D printing questions."
		}

		msgText := systemPrompt + "\n\n" + req.Message
		if req.Context != "" {
			msgText += "\n\n" + req.Context
		}

		// Save user message
		userMsg := ai.ChatMessage{
			Role:      "user",
			Text:      msgText,
			Timestamp: time.Now(),
		}
		if len(imagePaths) > 0 {
			userMsg.HasImage = true
			userMsg.ImagePaths = imagePaths
			userMsg.ImageMime = "image/jpeg"
		}
		_ = chatStore.AddMessage(conv.ID, userMsg)

		// Build Gemini request
		conv = chatStore.Get(conv.ID)
		var geminiMsgs []ai.ChatMessageForAPI
		for _, msg := range conv.Messages {
			parts := []ai.ChatPart{{Text: msg.Text}}
			if msg.HasImage {
				for _, imgPath := range msg.ImagePaths {
					imgData, err := os.ReadFile(chatStore.ImagePath(imgPath))
					if err != nil {
						continue
					}
					parts = append(parts, ai.ChatPart{
						InlineData: &struct {
							MimeType string `json:"mimeType"`
							Data     string `json:"data"`
						}{
							MimeType: msg.ImageMime,
							Data:     ai.EncodeImageBase64(imgData),
						},
					})
				}
			}
			geminiMsgs = append(geminiMsgs, ai.ChatMessageForAPI{
				Role:  msg.Role,
				Parts: parts,
			})
		}

		chatReq := ai.ChatRequest{
			APIKey:   apiKey,
			Messages: geminiMsgs,
		}
		result, err := ai.Chat(chatReq)
		if err != nil {
			log.Printf("[ai] analyze failed: %v", err)
			_ = chatStore.AddMessage(conv.ID, ai.ChatMessage{
				Role:      "model",
				Text:      fmt.Sprintf("Error: %s", err.Error()),
				Timestamp: time.Now(),
			})
			errJSON, _ := json.Marshal(map[string]string{"error": err.Error()})
			http.Error(w, string(errJSON), http.StatusInternalServerError)
			return
		}

		_ = chatStore.AddMessage(conv.ID, ai.ChatMessage{
			Role:      "model",
			Text:      result.Text,
			Timestamp: time.Now(),
		})

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(chatStore.Get(conv.ID))
	})

	// Start a new empty chat conversation, optionally linked to a printer.
	mux.HandleFunc("/api/ai/chat/start", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			PrinterID   string `json:"printerId"`
			PrinterName string `json:"printerName"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)

		conv := chatStore.Create(req.PrinterID, req.PrinterName, "")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(conv)
	})

	// captureSnapshot grabs the current camera frame(s) + printer data for
	// the given printer and returns them as a text context string + saved
	// image paths. It does NOT call Gemini — the caller includes the result
	// in the next message.
	captureSnapshot := func(convID, printerID string) (contextText string, imagePaths []string) {
		currentMgr := mgr.Load()
		var temps printers.Temps
		var progress int
		var currentFile string
		var layerNum, layerCount int
		var printerName string
		for _, st := range currentMgr.Statuses() {
			if st.ID == printerID {
				temps = st.Temps
				progress = st.Progress
				currentFile = st.CurrentFile
				layerNum = st.LayerNum
				layerCount = st.LayerCount
				printerName = st.Name
				break
			}
		}

		// Capture frames from cameras assigned to this printer
		type capturedFrame struct {
			CameraName string
			Data       []byte
			RelPath    string
		}
		var frames []capturedFrame
		for _, cam := range cameraMgr.Store().GetCameras() {
			if !cam.Enabled || cam.PrinterID != printerID {
				continue
			}
			if cam.Type != "usb" && cam.Type != "rpicam" {
				continue
			}
			frameData := cameraMgr.Streamers().GetFrameForCamera(cam.ID)
			if frameData == nil {
				continue
			}
			frames = append(frames, capturedFrame{
				CameraName: cam.Name,
				Data:       frameData,
			})
		}

		// Build context text
		var sb strings.Builder
		sb.WriteString("\n\n## Live Snapshot Data\n")
		if printerName != "" {
			sb.WriteString(fmt.Sprintf("- Printer: %s\n", printerName))
		}
		if currentFile != "" {
			sb.WriteString(fmt.Sprintf("- File: %s\n", currentFile))
		}
		sb.WriteString(fmt.Sprintf("- Progress: %d%%\n", progress))
		if layerCount > 0 {
			sb.WriteString(fmt.Sprintf("- Layer: %d / %d\n", layerNum, layerCount))
		}
		sb.WriteString(fmt.Sprintf("- Nozzle: %.1f°C (target: %.1f°C)\n", temps.Nozzle, temps.TargetNozzle))
		sb.WriteString(fmt.Sprintf("- Bed: %.1f°C (target: %.1f°C)\n", temps.Bed, temps.TargetBed))
		if len(frames) > 0 {
			sb.WriteString(fmt.Sprintf("- Cameras: %d frame(s) attached\n", len(frames)))
			for i, f := range frames {
				sb.WriteString(fmt.Sprintf("  - Frame %d: %s\n", i+1, f.CameraName))
			}
		} else {
			sb.WriteString("- Cameras: no frames available\n")
		}

		// Save images
		for _, f := range frames {
			relPath, err := chatStore.SaveImage(convID, f.Data, ".jpg")
			if err != nil {
				continue
			}
			f.RelPath = relPath
			imagePaths = append(imagePaths, relPath)
		}

		return sb.String(), imagePaths
	}

	// Send a message in an existing chat. If attachSnapshot is true and a
	// printerId is provided, captures the live frame + printer data and
	// includes it with the message.
	mux.HandleFunc("/api/ai/chat/{id}/send", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		convID := r.PathValue("id")
		conv := chatStore.Get(convID)
		if conv == nil {
			http.Error(w, `{"error":"conversation not found"}`, http.StatusNotFound)
			return
		}

		var req struct {
			Message        string `json:"message"`
			AttachSnapshot bool   `json:"attachSnapshot"`
			PrinterID      string `json:"printerId"`
			GcodeFileID    string `json:"gcodeFileId"`
			GcodeFileName  string `json:"gcodeFileName"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
			return
		}
		if req.Message == "" && !req.AttachSnapshot && req.GcodeFileID == "" {
			http.Error(w, `{"error":"message, snapshot, or gcode file required"}`, http.StatusBadRequest)
			return
		}

		apiKey := resolveAPIKey(settingsFile)
		if apiKey == "" {
			http.Error(w, `{"error":"no Gemini API key configured"}`, http.StatusBadRequest)
			return
		}

		// Build the user message text
		msgText := req.Message
		var imagePaths []string
		if req.AttachSnapshot && req.PrinterID != "" {
			snapshotText, paths := captureSnapshot(convID, req.PrinterID)
			msgText += snapshotText
			imagePaths = paths
		}

		// Attach gcode file content if requested
		if req.GcodeFileID != "" {
			gcodeData, err := gcodeStore.Load(req.GcodeFileID)
			if err != nil {
				errJSON, _ := json.Marshal(map[string]string{"error": fmt.Sprintf("failed to load gcode: %s", err.Error())})
				http.Error(w, string(errJSON), http.StatusBadRequest)
				return
			}
			gcodeText := string(gcodeData)
			// Truncate very large gcode files to avoid exceeding token limits
			maxGcode := 50000
			truncated := false
			if len(gcodeText) > maxGcode {
				gcodeText = gcodeText[:maxGcode]
				truncated = true
			}
			fileName := req.GcodeFileName
			if fileName == "" {
				fileName = req.GcodeFileID
			}
			msgText += fmt.Sprintf("\n\n--- G-code file: %s ---\n%s", fileName, gcodeText)
			if truncated {
				msgText += "\n... (truncated, file too large for full analysis)"
			}
			msgText += "\n--- End G-code ---"
		}

		// If this is the first message, prepend the system prompt
		isFirst := len(conv.Messages) == 0
		if isFirst {
			msgText = "You are a 3D printing expert assistant. The user can attach live camera snapshots from their printers along with temperature and progress data. Analyze the images and data to help identify issues, suggest fixes, and answer questions about their prints.\n\n" + msgText
		}

		// Save user message
		userMsg := ai.ChatMessage{
			Role:      "user",
			Text:      msgText,
			Timestamp: time.Now(),
		}
		if len(imagePaths) > 0 {
			userMsg.HasImage = true
			userMsg.ImagePaths = imagePaths
			userMsg.ImageMime = "image/jpeg"
		}
		_ = chatStore.AddMessage(convID, userMsg)

		// Rebuild full conversation history for Gemini
		conv = chatStore.Get(convID)
		var geminiMsgs []ai.ChatMessageForAPI
		for _, msg := range conv.Messages {
			parts := []ai.ChatPart{{Text: msg.Text}}
			if msg.HasImage {
				for _, imgPath := range msg.ImagePaths {
					imgData, err := os.ReadFile(chatStore.ImagePath(imgPath))
					if err != nil {
						continue
					}
					parts = append(parts, ai.ChatPart{
						InlineData: &struct {
							MimeType string `json:"mimeType"`
							Data     string `json:"data"`
						}{
							MimeType: msg.ImageMime,
							Data:     ai.EncodeImageBase64(imgData),
						},
					})
				}
			}
			geminiMsgs = append(geminiMsgs, ai.ChatMessageForAPI{
				Role:  msg.Role,
				Parts: parts,
			})
		}

		chatReq := ai.ChatRequest{
			APIKey:   apiKey,
			Messages: geminiMsgs,
		}
		result, err := ai.Chat(chatReq)
		if err != nil {
			log.Printf("[ai] chat send failed: %v", err)
			errMsg := err.Error()
			_ = chatStore.AddMessage(convID, ai.ChatMessage{
				Role:      "model",
				Text:      fmt.Sprintf("Error: %s", errMsg),
				Timestamp: time.Now(),
			})
			errJSON, _ := json.Marshal(map[string]string{"error": errMsg})
			http.Error(w, string(errJSON), http.StatusInternalServerError)
			return
		}

		_ = chatStore.AddMessage(convID, ai.ChatMessage{
			Role:      "model",
			Text:      result.Text,
			Timestamp: time.Now(),
		})

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(chatStore.Get(convID))
	})

	// Get a conversation
	mux.HandleFunc("/api/ai/chat/{id}", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		convID := r.PathValue("id")
		conv := chatStore.Get(convID)
		if conv == nil {
			http.Error(w, `{"error":"conversation not found"}`, http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(conv)
	})

	// List all conversations
	mux.HandleFunc("/api/ai/chat", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(chatStore.List())
	})

	// Delete a conversation
	mux.HandleFunc("/api/ai/chat/{id}/delete", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		convID := r.PathValue("id")
		if err := chatStore.Delete(convID); err != nil {
			http.Error(w, `{"error":"conversation not found"}`, http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
	})

	// Serve a captured frame image from a chat conversation
	mux.HandleFunc("/api/ai/chat/{id}/image", func(w http.ResponseWriter, r *http.Request) {
		convID := r.PathValue("id")
		imgPath := r.URL.Query().Get("path")
		if imgPath == "" {
			http.Error(w, `{"error":"path required"}`, http.StatusBadRequest)
			return
		}
		// Verify the image belongs to this conversation
		conv := chatStore.Get(convID)
		if conv == nil {
			http.Error(w, `{"error":"conversation not found"}`, http.StatusNotFound)
			return
		}
		found := false
		for _, msg := range conv.Messages {
			for _, p := range msg.ImagePaths {
				if p == imgPath {
					found = true
					break
				}
			}
			if found {
				break
			}
		}
		if !found {
			http.Error(w, `{"error":"image not found in conversation"}`, http.StatusNotFound)
			return
		}
		fullPath := chatStore.ImagePath(imgPath)
		w.Header().Set("Content-Type", "image/jpeg")
		http.ServeFile(w, r, fullPath)
	})

	cameras.Mount(mux, cameraMgr)
	pi.Mount(mux, piMgr)
	mux.HandleFunc("/api/config", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			// Start with settings.json (or empty defaults)
			var config map[string]any
			if data, err := os.ReadFile(settingsFile); err == nil {
				_ = json.Unmarshal(data, &config)
			}
			if config == nil {
				config = map[string]any{}
			}

			// Merge env-based secrets (env takes precedence over settings.json).
			// Only merge non-empty string values — boolean false from env
			// defaults must not override saved true values in settings.json.
			envSecrets := envSecretConfig()
			for k, v := range envSecrets {
				if s, ok := v.(string); ok {
					if s != "" {
						config[k] = v
					}
				} else {
					// Non-string values (booleans) are only present when
					// explicitly set, so always merge them.
					config[k] = v
				}
			}

			_ = json.NewEncoder(w).Encode(config)
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
				AuthPasscode string `json:"authPasscode"`
			}
			_ = json.Unmarshal(body, &openpolyprintSettings)
			for id, i := range openpolyprintSettings.Integrations {
				intgMgr.SetConfig(id, i.Fields)
			}
			// Update auth passcode if changed (env takes precedence — don't override env)
			if envconfig.Get("AUTH_PASSCODE", "") == "" {
				authMgr.SetPasscode(openpolyprintSettings.AuthPasscode)
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

	// â”€â”€â”€ OctoPrint-compatible API â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€
	// These endpoints allow slicers (PrusaSlicer, OrcaSlicer, Cura) to upload
	// G-code and start prints. Routing to a specific printer is done via:
	//   1. POST /api/files/{printerName}/local â€” explicit printer in path
	//   2. POST /api/files/local â€” uses the configured "slicer target" printer

	// corsMiddleware adds CORS headers for browser-based slicer plugins.
	corsMiddleware := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Api-Key")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next(w, r)
		}
	}

	mux.HandleFunc("/api/version", corsMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"api":    "0.1",
			"server": "1.9.0",
			"text":   "OctoPrint 1.9.0",
		})
	}))

	// /api/login — OctoPrint passive login. Always returns success with no
	// session since OpenPolyPrint doesn't require API keys for slicer uploads.
	mux.HandleFunc("/api/login", corsMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"_is_external_client": true,
			"session":             "openpolyprint",
			"username":            "openpolyprint",
		})
	}))

	// /api/printerprofiles — return a default profile
	mux.HandleFunc("/api/printerprofiles", corsMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"profiles": []map[string]any{
				{"id": "default", "name": "OpenPolyPrint", "default": true},
			},
		})
	}))

	mux.HandleFunc("/api/connection", corsMiddleware(func(w http.ResponseWriter, r *http.Request) {
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
	}))

	mux.HandleFunc("/api/printer", corsMiddleware(func(w http.ResponseWriter, r *http.Request) {
		// Return status of the first available printer (for OctoPrint compat).
		var status printers.Status
		statuses := mgr.Load().Statuses()
		if len(statuses) > 0 {
			status = statuses[0]
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
			"state": map[string]any{
				"text": stateText,
				"flags": map[string]any{
					"operational":   stateText != "Offline",
					"printing":      stateText == "Printing",
					"closedOrError": stateText == "Offline",
					"error":         false,
					"paused":        stateText == "Paused",
					"pausing":       false,
					"cancelling":    false,
					"ready":         stateText == "Operational",
					"sdReady":       false,
				},
			},
			"temperature": map[string]any{
				"tool0": map[string]float64{"actual": status.Temps.Nozzle, "target": status.Temps.TargetNozzle, "offset": 0},
				"bed":   map[string]float64{"actual": status.Temps.Bed, "target": status.Temps.TargetBed, "offset": 0},
			},
		})
	}))

	mux.HandleFunc("/api/job", corsMiddleware(func(w http.ResponseWriter, r *http.Request) {
		var status printers.Status
		statuses := mgr.Load().Statuses()
		if len(statuses) > 0 {
			status = statuses[0]
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
	}))

	// Stub endpoints that some slicers probe to verify OctoPrint connectivity
	mux.HandleFunc("/api/settings", corsMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"api":     map[string]any{"enabled": true, "key": ""},
			"feature": map[string]any{"gcodeViewer": true},
			"folder":  map[string]string{"uploads": "/uploads", "timelapse": "/timelapses"},
			"webcam":  map[string]any{"streamUrl": "/api/cameras", "ffmpeg": true},
		})
	}))

	mux.HandleFunc("/api/timelapse", corsMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"config": map[string]string{"type": "off"},
			"files":  []any{},
		})
	}))

	// /api/files (without trailing slash) â€” return empty file list
	mux.HandleFunc("/api/files", corsMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"files": []any{},
		})
	}))

	// OctoPrint file upload: POST /api/files/local
	// Saves the G-code to the G-code store (unassigned). The user then
	// assigns it to a printer in the OpenPolyPrint UI.
	mux.HandleFunc("/api/files/", func(w http.ResponseWriter, r *http.Request) {
		// CORS for browser-based slicer plugins (e.g. Cura)
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Api-Key")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		// GET /api/files or /api/files/local — return empty file list
		// (slicers often probe this before uploading)
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"files": []any{},
			})
			return
		}

		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		// Parse path. Only standard OctoPrint paths:
		//   POST /api/files/local            → upload (save to G-code store, unassigned)
		//   POST /api/files/local/{filename} → select/start print (accept silently)
		pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/files/"), "/")
		isUpload := len(pathParts) == 1 && pathParts[0] == "local"

		if isUpload {
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

			filename := header.Filename
			if filename == "" {
				filename = "upload.gcode"
			}

			// Save to G-code store as unassigned (no printer ID).
			// The user will assign it to a printer in the OpenPolyPrint UI.
			if _, err := gcodeStore.Save(filename, "", bytes.NewReader(data)); err != nil {
				log.Printf("[slicer] save %s to gcode store failed: %v", filename, err)
				http.Error(w, fmt.Sprintf(`{"error":"save failed: %s"}`, err.Error()), http.StatusInternalServerError)
				return
			}
			log.Printf("[slicer] saved %s (%d bytes) to gcode store (unassigned)", filename, len(data))

			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Location", "/api/files/local/"+filename)
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"files": map[string]any{
					"local": map[string]any{
						"name":   filename,
						"path":   filename,
						"origin": "local",
						"size":   len(data),
						"type":   "machinecode",
						"date":   time.Now().Unix(),
					},
				},
				"done": true,
			})
			return
		}

		// File select: POST /api/files/local/{filename}
		// with JSON body {"command":"select","print":true}
		// Accept silently — the user will start the print from the OpenPolyPrint UI.
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{})
	})

	// â”€â”€â”€ Smart Plug API â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€
	mux.HandleFunc("/api/push/vapid-key", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"key": pushMgr.VapidPublicKey()})
	})

	mux.HandleFunc("/api/push/subscribe", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		var sub push.Subscription
		if err := json.NewDecoder(r.Body).Decode(&sub); err != nil {
			http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
			return
		}
		pushMgr.AddSubscription(sub)
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("/api/push/unsubscribe", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Endpoint string `json:"endpoint"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		pushMgr.RemoveSubscription(req.Endpoint)
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("/api/plugs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(plugMgr.List())
		case http.MethodPost:
			var plug smartplug.Plug
			if err := json.NewDecoder(r.Body).Decode(&plug); err != nil {
				http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
				return
			}
			created := plugMgr.Add(plug)
			_ = json.NewEncoder(w).Encode(created)
		default:
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/plugs/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/api/plugs/")
		if id == "" {
			http.Error(w, `{"error":"id required"}`, http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodDelete:
			if !plugMgr.Remove(id) {
				http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		case http.MethodPost:
			var req struct {
				On bool `json:"on"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			if err := plugMgr.SetOn(id, req.On); err != nil {
				log.Printf("[smartplug] set %s on=%v failed: %v", id, req.On, err)
				http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
		default:
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	// â”€â”€â”€ Filament API â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€
	mux.HandleFunc("/api/filament", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(filamentStore.List())
		case http.MethodPost:
			var spool filament.Spool
			if err := json.NewDecoder(r.Body).Decode(&spool); err != nil {
				http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
				return
			}
			created := filamentStore.Add(spool)
			_ = json.NewEncoder(w).Encode(created)
		default:
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/filament/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/api/filament/")
		if id == "" {
			http.Error(w, `{"error":"id required"}`, http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodPut:
			var spool filament.Spool
			if err := json.NewDecoder(r.Body).Decode(&spool); err != nil {
				http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
				return
			}
			if !filamentStore.Update(id, spool) {
				http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
				return
			}
			_ = json.NewEncoder(w).Encode(spool)
		case http.MethodDelete:
			if !filamentStore.Remove(id) {
				http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	// ── Print Profiles API ─────────────────────────────────────────────
	mux.HandleFunc("/api/profiles", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(profileStore.List())
		case http.MethodPost:
			var p profiles.Profile
			if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
				http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(profileStore.Add(p))
		default:
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/profiles/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodPut:
			var p profiles.Profile
			if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
				http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
				return
			}
			if !profileStore.Update(id, p) {
				http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
				return
			}
			_ = json.NewEncoder(w).Encode(p)
		case http.MethodDelete:
			if !profileStore.Remove(id) {
				http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	// ── Profile Files API (uploaded slicer profiles) ───────────────────
	mux.HandleFunc("/api/profile-files", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			category := profilefiles.Category(r.URL.Query().Get("category"))
			_ = json.NewEncoder(w).Encode(profileFilesStore.List(category))
		case http.MethodPost:
			// Multipart upload: file + metadata fields
			if err := r.ParseMultipartForm(32 << 20); err != nil {
				http.Error(w, `{"error":"failed to parse form: `+err.Error()+`"}`, http.StatusBadRequest)
				return
			}
			file, header, err := r.FormFile("file")
			if err != nil {
				http.Error(w, `{"error":"no file provided"}`, http.StatusBadRequest)
				return
			}
			defer file.Close()
			content, err := io.ReadAll(file)
			if err != nil {
				http.Error(w, `{"error":"failed to read file"}`, http.StatusBadRequest)
				return
			}
			name := r.FormValue("name")
			if name == "" {
				name = header.Filename
			}
			category := profilefiles.Category(r.FormValue("category"))
			if category != profilefiles.CategoryFilament && category != profilefiles.CategoryPrint {
				http.Error(w, `{"error":"invalid category"}`, http.StatusBadRequest)
				return
			}
			slicer := r.FormValue("slicer")
			notes := r.FormValue("notes")
			var tags []string
			if tagsStr := r.FormValue("tags"); tagsStr != "" {
				for _, t := range strings.Split(tagsStr, ",") {
					t = strings.TrimSpace(t)
					if t != "" {
						tags = append(tags, t)
					}
				}
			}
			pf, err := profileFilesStore.Add(name, header.Filename, category, content, slicer, tags, notes)
			if err != nil {
				http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
				return
			}
			_ = json.NewEncoder(w).Encode(pf)
		default:
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/profile-files/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		switch r.Method {
		case http.MethodGet:
			// Download the file content
			data, filename, err := profileFilesStore.Content(id)
			if err != nil {
				http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
			_, _ = w.Write(data)
		case http.MethodPut, http.MethodPatch:
			// Update metadata (name, tags, notes, slicer)
			var body struct {
				Name   string   `json:"name"`
				Tags   []string `json:"tags"`
				Notes  string   `json:"notes"`
				Slicer string   `json:"slicer"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
				return
			}
			if !profileFilesStore.Update(id, body.Name, body.Tags, body.Notes, body.Slicer) {
				http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			pf, _ := profileFilesStore.Get(id)
			_ = json.NewEncoder(w).Encode(pf)
		case http.MethodDelete:
			if !profileFilesStore.Remove(id) {
				http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/profile-files/{id}/view", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		id := r.PathValue("id")
		pf, err := profileFilesStore.Get(id)
		if err != nil {
			http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(pf)
	})

	mux.HandleFunc("/api/profile-files/tags", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		category := profilefiles.Category(r.URL.Query().Get("category"))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(profileFilesStore.AllTags(category))
	})

	// ── Profile Converter API ──────────────────────────────────────────
	mux.HandleFunc("/api/profile-files/convert", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		// Accept either multipart (file upload) or JSON (convert existing file by ID)
		contentType := r.Header.Get("Content-Type")
		if strings.HasPrefix(contentType, "multipart/") {
			if err := r.ParseMultipartForm(32 << 20); err != nil {
				http.Error(w, `{"error":"failed to parse form"}`, http.StatusBadRequest)
				return
			}
			file, header, err := r.FormFile("file")
			if err != nil {
				http.Error(w, `{"error":"no file provided"}`, http.StatusBadRequest)
				return
			}
			defer file.Close()
			content, err := io.ReadAll(file)
			if err != nil {
				http.Error(w, `{"error":"failed to read file"}`, http.StatusBadRequest)
				return
			}
			target := profileconverter.Format(r.FormValue("target"))
			if target != profileconverter.FormatCura && target != profileconverter.FormatPrusaSlicer && target != profileconverter.FormatOrcaSlicer {
				http.Error(w, `{"error":"invalid target format"}`, http.StatusBadRequest)
				return
			}
			result, err := profileconverter.Convert(string(content), header.Filename, target)
			if err != nil {
				http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
				return
			}
			// Optionally save the converted file
			if r.FormValue("save") == "true" {
				category := profilefiles.Category(r.FormValue("category"))
				if category == "" {
					category = profilefiles.CategoryPrint
				}
				slicer := "prusaslicer"
				if target == profileconverter.FormatCura {
					slicer = "cura"
				} else if target == profileconverter.FormatOrcaSlicer {
					slicer = "orcaslicer"
				}
				var tags []string
				if tagsStr := r.FormValue("tags"); tagsStr != "" {
					for _, t := range strings.Split(tagsStr, ",") {
						t = strings.TrimSpace(t)
						if t != "" {
							tags = append(tags, t)
						}
					}
				}
				pf, err := profileFilesStore.Add(
					strings.TrimSuffix(result.Filename, filepath.Ext(result.Filename)),
					result.Filename,
					category,
					[]byte(result.Content),
					slicer,
					tags,
					"Converted by OpenPolyPrint",
				)
				if err == nil {
					result.SavedID = pf.ID
				}
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(result)
		} else {
			// JSON body: convert an existing stored file by ID
			var body struct {
				ID       string `json:"id"`
				Target   string `json:"target"`
				Save     bool   `json:"save"`
				Category string `json:"category"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
				return
			}
			pf, err := profileFilesStore.Get(body.ID)
			if err != nil {
				http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
				return
			}
			target := profileconverter.Format(body.Target)
			if target != profileconverter.FormatCura && target != profileconverter.FormatPrusaSlicer && target != profileconverter.FormatOrcaSlicer {
				http.Error(w, `{"error":"invalid target format"}`, http.StatusBadRequest)
				return
			}
			result, err := profileconverter.Convert(pf.Content, pf.Filename, target)
			if err != nil {
				http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
				return
			}
			if body.Save {
				category := profilefiles.Category(body.Category)
				if category == "" {
					category = pf.Category
				}
				slicer := "prusaslicer"
				if target == profileconverter.FormatCura {
					slicer = "cura"
				} else if target == profileconverter.FormatOrcaSlicer {
					slicer = "orcaslicer"
				}
				saved, err := profileFilesStore.Add(
					strings.TrimSuffix(result.Filename, filepath.Ext(result.Filename)),
					result.Filename,
					category,
					[]byte(result.Content),
					slicer,
					pf.Tags,
					"Converted from "+pf.Name,
				)
				if err == nil {
					result.SavedID = saved.ID
				}
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(result)
		}
	})

	// ── STL Files API ─────────────────────────────────────────────────
	mux.HandleFunc("/api/stl-files", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(stlFilesStore.List())
		case http.MethodPost:
			if err := r.ParseMultipartForm(100 << 20); err != nil {
				http.Error(w, `{"error":"failed to parse form"}`, http.StatusBadRequest)
				return
			}
			h := r.MultipartForm.File["file"]
			if len(h) == 0 {
				http.Error(w, `{"error":"no file"}`, http.StatusBadRequest)
				return
			}
			f, err := h[0].Open()
			if err != nil {
				http.Error(w, `{"error":"failed to open file"}`, http.StatusInternalServerError)
				return
			}
			defer f.Close()
			data, err := io.ReadAll(f)
			if err != nil {
				http.Error(w, `{"error":"failed to read file"}`, http.StatusInternalServerError)
				return
			}
			name := h[0].Filename
			if name == "" {
				name = "model.stl"
			}
			displayName := strings.TrimSuffix(name, filepath.Ext(name))
			var tags []string
			if vals := r.MultipartForm.Value["tags"]; len(vals) > 0 {
				for _, t := range strings.Split(vals[0], ",") {
					t = strings.TrimSpace(t)
					if t != "" {
						tags = append(tags, t)
					}
				}
			}
			notes := ""
			if vals := r.MultipartForm.Value["notes"]; len(vals) > 0 {
				notes = vals[0]
			}
			saved, err := stlFilesStore.Add(displayName, name, data, tags, notes)
			if err != nil {
				http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
				return
			}
			_ = json.NewEncoder(w).Encode(saved)
		default:
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/stl-files/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		switch r.Method {
		case http.MethodGet:
			data, filename, err := stlFilesStore.Content(id)
			if err != nil {
				http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
			w.Write(data)
		case http.MethodPut:
			var body struct {
				Name     string   `json:"name"`
				Filename string   `json:"filename"`
				Tags     []string `json:"tags"`
				Notes    string   `json:"notes"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
				return
			}
			if !stlFilesStore.Update(id, body.Name, body.Filename, body.Tags, body.Notes) {
				http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
		case http.MethodDelete:
			if !stlFilesStore.Remove(id) {
				http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/stl-files/tags", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(stlFilesStore.AllTags())
	})

	// ── Maintenance API ────────────────────────────────────────────────
	mux.HandleFunc("/api/maintenance", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			// Compute statuses with empty print hours for now
			_ = json.NewEncoder(w).Encode(maintStore.Statuses(nil))
		case http.MethodPost:
			var rem maintenance.Reminder
			if err := json.NewDecoder(r.Body).Decode(&rem); err != nil {
				http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(maintStore.Add(rem))
		default:
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/maintenance/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodPut:
			var rem maintenance.Reminder
			if err := json.NewDecoder(r.Body).Decode(&rem); err != nil {
				http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
				return
			}
			if !maintStore.Update(id, rem) {
				http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
				return
			}
			_ = json.NewEncoder(w).Encode(rem)
		case http.MethodDelete:
			if !maintStore.Remove(id) {
				http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		case http.MethodPost:
			// Mark as performed
			if !maintStore.MarkPerformed(id) {
				http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
		default:
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/temps", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(tempStore.GetAll())
	})

	// â”€â”€â”€ Print Queue API â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€
	mux.HandleFunc("/api/queue", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(queueStore.List())
		case http.MethodPost:
			var req struct {
				PrinterID string `json:"printerId"`
				Filename  string `json:"filename"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
				return
			}
			if req.PrinterID == "" || req.Filename == "" {
				http.Error(w, `{"error":"printerId and filename required"}`, http.StatusBadRequest)
				return
			}
			item := queueStore.Add(req.PrinterID, req.Filename)
			_ = json.NewEncoder(w).Encode(item)
		case http.MethodDelete:
			queueStore.ClearAll()
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/queue/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/api/queue/")
		if id == "" {
			http.Error(w, `{"error":"id required"}`, http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodDelete:
			queueStore.Remove(id)
			w.WriteHeader(http.StatusNoContent)
		case http.MethodPost:
			// Start a queued item manually
			if d := mgr.Load().Find(id); d != nil {
				// Find the queue item by looking through the list
				for _, item := range queueStore.List() {
					if item.ID == id && item.Status == "pending" {
						queueStore.UpdateStatus(id, "printing", "")
						if err := d.StartPrint(r.Context(), item.Filename); err != nil {
							queueStore.UpdateStatus(id, "failed", err.Error())
							http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
							return
						}
						_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
						return
					}
				}
			}
			http.Error(w, `{"error":"queue item not found or printer unavailable"}`, http.StatusNotFound)
		default:
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/temps/", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id == "" {
			id = strings.TrimPrefix(r.URL.Path, "/api/temps/")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(tempStore.Get(id))
	})

	mux.HandleFunc("/api/printers", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(applyAliases(mgr.Load().Statuses()))
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
			_ = json.NewEncoder(w).Encode(applyAliases(mgr.Load().Statuses()))
		default:
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/printers/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		switch r.Method {
		case http.MethodPut, http.MethodPatch:
			// Rename printer (set custom alias)
			var body struct {
				Name string `json:"name"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
				return
			}
			printerAliasesMu.Lock()
			if printerAliases == nil {
				printerAliases = map[string]string{}
			}
			if body.Name == "" {
				delete(printerAliases, id)
			} else {
				printerAliases[id] = body.Name
			}
			if err := savePrinterAliases(filepath.Join(settingsDir, "printer_aliases.json"), printerAliases); err != nil {
				printerAliasesMu.Unlock()
				http.Error(w, `{"error":"failed to save alias"}`, http.StatusInternalServerError)
				return
			}
			printerAliasesMu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(applyAliases(mgr.Load().Statuses()))
		case http.MethodDelete:
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
		default:
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		}
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

	// ---- Print session endpoints (AI data collection) ----
	mux.HandleFunc("/api/printers/{id}/session", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		switch r.Method {
		case http.MethodGet:
			// Return active session info + temp data
			sess := sessMgr.Get(id)
			if sess == nil {
				http.Error(w, `{"error":"no active session"}`, http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(sess)
		case http.MethodPost:
			// Manually start a session
			var req struct {
				FileName string `json:"fileName"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			m := mgr.Load()
			if m == nil {
				http.Error(w, `{"error":"manager not available"}`, http.StatusInternalServerError)
				return
			}
			d := m.Find(id)
			if d == nil {
				http.Error(w, `{"error":"printer not found"}`, http.StatusNotFound)
				return
			}
			name := id
			if s, err := d.Status(); err == nil {
				name = s.Name
				if req.FileName == "" {
					req.FileName = s.CurrentFile
				}
			}
			sess := sessMgr.Start(id, name, req.FileName)
			if sess == nil {
				http.Error(w, `{"error":"failed to start session"}`, http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "session": sess})
		case http.MethodDelete:
			// Stop the session
			sess := sessMgr.Stop(id, "Stopped")
			if sess == nil {
				http.Error(w, `{"error":"no active session"}`, http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "session": sess})
		default:
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/printers/sessions", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"sessions": sessMgr.ActiveSessions()})
	})

	// ---- Saved session endpoints (for AI analysis page) ----
	mux.HandleFunc("/api/sessions", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		sessions, err := sessMgr.ListSavedSessions()
		if err != nil {
			http.Error(w, `{"error":"failed to list sessions"}`, http.StatusInternalServerError)
			return
		}
		// Check for matching gcode files
		gcodeFiles, _ := gcodeStore.List()
		gcodeByName := make(map[string]string) // name -> ID
		for _, f := range gcodeFiles {
			gcodeByName[f.Name] = f.ID
		}
		for i := range sessions {
			if id, ok := gcodeByName[sessions[i].FileName]; ok {
				sessions[i].HasGcode = true
				sessions[i].GcodeID = id
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"sessions": sessions})
	})

	mux.HandleFunc("/api/sessions/{id}", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		id := r.PathValue("id")
		sess, err := sessMgr.GetSavedSession(id)
		if err != nil {
			http.Error(w, `{"error":"session not found"}`, http.StatusNotFound)
			return
		}
		// Include timelapse dir and gcode ID
		timelapseDir := ""
		timestamp := sess.StartTime.Format("20060102-150405")
		if tlEntries, err := os.ReadDir("recordings/timelapse"); err == nil {
			for _, e := range tlEntries {
				if e.IsDir() && strings.HasSuffix(e.Name(), "_frames") && strings.Contains(e.Name(), timestamp) {
					timelapseDir = e.Name()
					break
				}
			}
		}
		gcodeID := ""
		gcodeFiles, _ := gcodeStore.List()
		for _, f := range gcodeFiles {
			if f.Name == sess.FileName {
				gcodeID = f.ID
				break
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"session":      sess,
			"timelapseDir": timelapseDir,
			"gcodeId":      gcodeID,
		})
	})

	// ---- Manual recording per printer (video or timelapse) ----
	mux.HandleFunc("/api/printers/{id}/record/start", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		id := r.PathValue("id")
		var req struct {
			Mode     string  `json:"mode"`     // "video" or "timelapse"
			Interval float64 `json:"interval"` // for timelapse
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Mode != "video" && req.Mode != "timelapse" {
			req.Mode = "video"
		}

		// Find a camera assigned to this printer
		var camID string
		for _, cam := range cameraMgr.Store().GetCameras() {
			if cam.PrinterID == id && (cam.Type == "usb" || cam.Type == "rpicam") && cam.Enabled {
				camID = cam.ID
				break
			}
		}
		if camID == "" {
			http.Error(w, `{"error":"no camera assigned to this printer"}`, http.StatusBadRequest)
			return
		}

		streamer := cameraMgr.Streamers().GetStream(camID)
		if streamer == nil {
			// Start the stream if not running
			for _, cam := range cameraMgr.Store().GetCameras() {
				if cam.ID == camID {
					cameraMgr.Streamers().StartStream(&cam)
					break
				}
			}
			time.Sleep(1500 * time.Millisecond)
			streamer = cameraMgr.Streamers().GetStream(camID)
		}
		if streamer == nil {
			http.Error(w, `{"error":"camera stream not available"}`, http.StatusInternalServerError)
			return
		}

		// Get printer name and file for filename
		printerName := id
		fileName := ""
		if m := mgr.Load(); m != nil {
			if d := m.Find(id); d != nil {
				if s, err := d.Status(); err == nil {
					printerName = s.Name
					fileName = s.CurrentFile
				}
			}
		}
		filename := fmt.Sprintf("%s_%s_%s.mkv", safeName(printerName), safeName(fileName), time.Now().Format("20060102-150405"))

		var path string
		var err error
		if req.Mode == "timelapse" {
			if req.Interval <= 0 {
				req.Interval = 1
			}
			path, err = cameraMgr.Timelapses().Start(camID, streamer, filename, req.Interval)
		} else {
			path, err = cameraMgr.Records().Start(camID, streamer, filename)
		}
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "path": path, "mode": req.Mode, "cameraId": camID})
	})

	mux.HandleFunc("/api/printers/{id}/record/stop", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		id := r.PathValue("id")

		// Find camera for this printer and stop any active recording
		var camID string
		for _, cam := range cameraMgr.Store().GetCameras() {
			if cam.PrinterID == id && (cam.Type == "usb" || cam.Type == "rpicam") {
				camID = cam.ID
				break
			}
		}
		if camID == "" {
			http.Error(w, `{"error":"no camera assigned to this printer"}`, http.StatusBadRequest)
			return
		}

		var paths []string
		if cameraMgr.Records().IsRecording(camID) {
			if p, err := cameraMgr.Records().Stop(camID); err == nil {
				paths = append(paths, p)
			}
		}
		if cameraMgr.Timelapses().IsRecording(camID) {
			if p, err := cameraMgr.Timelapses().Stop(camID); err == nil {
				paths = append(paths, p)
			}
		}
		if len(paths) == 0 {
			http.Error(w, `{"error":"no active recording"}`, http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "paths": paths})
	})

	mux.HandleFunc("/api/printers/{id}/record/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		id := r.PathValue("id")
		var camID string
		for _, cam := range cameraMgr.Store().GetCameras() {
			if cam.PrinterID == id && (cam.Type == "usb" || cam.Type == "rpicam") {
				camID = cam.ID
				break
			}
		}
		status := map[string]any{
			"recording": false,
			"timelapse": false,
			"hasCamera": camID != "",
			"session":   sessMgr.IsActive(id),
		}
		if camID != "" {
			status["recording"] = cameraMgr.Records().IsRecording(camID)
			status["timelapse"] = cameraMgr.Timelapses().IsRecording(camID)
			status["videoStatus"] = cameraMgr.Records().Status(camID)
			status["timelapseStatus"] = cameraMgr.Timelapses().Status(camID)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(status)
	})

	mux.HandleFunc("/api/analytics", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(analytics.Compute(historyStore, filamentStore))
	})

	// ── Data Export ────────────────────────────────────────────────────
	mux.HandleFunc("/api/export", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		export := map[string]any{
			"exportedAt":  time.Now().Format(time.RFC3339),
			"version":     "1.0",
			"history":     historyStore.List(),
			"filament":    filamentStore.List(),
			"profiles":    profileStore.List(),
			"maintenance": maintStore.List(),
			"queue":       queueStore.List(),
			"analytics":   analytics.Compute(historyStore, filamentStore),
		}
		// Include settings (without secrets)
		if data, err := os.ReadFile(settingsFile); err == nil {
			var cfg map[string]any
			if json.Unmarshal(data, &cfg) == nil {
				// Strip sensitive keys
				delete(cfg, "geminiApiKey")
				delete(cfg, "authPasscode")
				export["settings"] = cfg
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", `attachment; filename="openpolyprint-export.json"`)
		_ = json.NewEncoder(w).Encode(export)
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

	go trackHistory(context.Background(), &mgr, cameraMgr, historyStore, settingsFile, intgMgr, tempStore, queueStore, plugMgr, pushMgr, sessMgr)

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
		// Serve favicon.ico — browsers request this automatically.
		// Redirect to the PNG icon since we don't have a .ico file.
		mux.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
			http.ServeFile(w, r, filepath.Join(dist, "icon-192.png"))
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

	// Wrap mux with auth middleware
	authedHandler := authMgr.Middleware(mux)

	// Periodically clean up expired sessions
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			authMgr.Cleanup()
		}
	}()

	// HTTP server (port 80) — always serves the app
	httpAddr := *addr
	if *enableTLS {
		// When TLS is on, the -addr port is for HTTPS; HTTP goes to :80
		httpAddr = ":80"
	}
	httpServer := &http.Server{Addr: httpAddr, Handler: authedHandler}
	go func() {
		fmt.Printf("OpenPolyPrint listening on http://localhost%s\n", httpAddr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("[http] server on %s: %v", httpAddr, err)
		}
	}()

	// HTTPS server — only when TLS is enabled
	var tlsServer *http.Server
	if *enableTLS {
		tlsAddr := *addr

		certDir := filepath.Join(settingsDir, "tls")
		certPath, keyPath, caCertPath, regenerated, err := tlsautocert.EnsureCertificate(certDir)
		if err != nil {
			log.Printf("[tls] failed to generate certificate, HTTPS disabled: %v", err)
		} else {
			if regenerated {
				log.Printf("[tls] certificate ready at %s (signed by local CA: %s)", certPath, caCertPath)
			}

			// Load the initial certificate for hot-swapping
			initialCert, err := tls.LoadX509KeyPair(certPath, keyPath)
			if err != nil {
				log.Printf("[tls] failed to load cert for hot-swap: %v", err)
			}

			// Auto-install CA into the local system trust store (where the app runs)
			if !tlsautocert.IsCAInstalledInSystemStore() {
				if err := tlsautocert.InstallCAToSystemStore(caCertPath); err != nil {
					log.Printf("[tls] could not auto-install CA to system store: %v (clients can install via /api/tls/install)", err)
				} else {
					log.Printf("[tls] CA auto-installed to local system trust store")
				}
			}

			// Endpoint to download the CA certificate
			mux.HandleFunc("/api/tls/ca", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/x-pem-file")
				w.Header().Set("Content-Disposition", "attachment; filename=openpolyprint-ca.pem")
				http.ServeFile(w, r, caCertPath)
			})

			// Endpoints to download platform-specific installer scripts
			mux.HandleFunc("/api/tls/install/windows", func(w http.ResponseWriter, r *http.Request) {
				host := r.Host
				if host == "" {
					host = "localhost"
				}
				// Strip port if present
				if h, _, err := net.SplitHostPort(host); err == nil {
					host = h
				}
				w.Header().Set("Content-Type", "application/octet-stream")
				w.Header().Set("Content-Disposition", "attachment; filename=install-openpolyprint-ca.bat")
				fmt.Fprint(w, tlsautocert.WindowsInstallScript(host))
			})

			mux.HandleFunc("/api/tls/install/mac", func(w http.ResponseWriter, r *http.Request) {
				host := r.Host
				if host == "" {
					host = "localhost"
				}
				if h, _, err := net.SplitHostPort(host); err == nil {
					host = h
				}
				w.Header().Set("Content-Type", "application/octet-stream")
				w.Header().Set("Content-Disposition", "attachment; filename=install-openpolyprint-ca.sh")
				fmt.Fprint(w, tlsautocert.MacInstallScript(host))
			})

			mux.HandleFunc("/api/tls/install/linux", func(w http.ResponseWriter, r *http.Request) {
				host := r.Host
				if host == "" {
					host = "localhost"
				}
				if h, _, err := net.SplitHostPort(host); err == nil {
					host = h
				}
				w.Header().Set("Content-Type", "application/octet-stream")
				w.Header().Set("Content-Disposition", "attachment; filename=install-openpolyprint-ca.sh")
				fmt.Fprint(w, tlsautocert.LinuxInstallScript(host))
			})

			// certPtr/certMu hold the current TLS certificate for hot-swapping
			// when new IP addresses are detected (Tailscale, VPN, etc.)
			var certMu sync.RWMutex
			certPtr := &initialCert

			tlsServer = &http.Server{Addr: tlsAddr, Handler: authedHandler}
			// Use GetCertificate so the cert can be hot-swapped when new IPs
			// are detected (e.g. Tailscale connects after the server starts).
			tlsServer.TLSConfig = &tls.Config{
				GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
					certMu.RLock()
					defer certMu.RUnlock()
					return certPtr, nil
				},
			}
			go func() {
				fmt.Printf("OpenPolyPrint listening on https://localhost%s\n", tlsAddr)
				fmt.Printf("  CA auto-installed on this host. For other devices:\n")
				fmt.Printf("    Windows: http://localhost/api/tls/install/windows\n")
				fmt.Printf("    macOS:   http://localhost/api/tls/install/mac\n")
				fmt.Printf("    Linux:   http://localhost/api/tls/install/linux\n")
				if err := tlsServer.ListenAndServeTLS(certPath, keyPath); err != nil && err != http.ErrServerClosed {
					log.Fatalf("[https] server on %s: %v", tlsAddr, err)
				}
			}()

			// Periodically check for new IP addresses (Tailscale, VPN, DHCP changes)
			// and regenerate the TLS certificate if needed. The cert is hot-swapped
			// via GetCertificate so there's no downtime.
			go func() {
				ticker := time.NewTicker(2 * time.Minute)
				defer ticker.Stop()
				for range ticker.C {
					regenerated, err := tlsautocert.CheckAndRegenerateIfNeeded(certDir)
					if err != nil {
						log.Printf("[tls] periodic check failed: %v", err)
						continue
					}
					if regenerated {
						newCert, err := tls.LoadX509KeyPair(certPath, keyPath)
						if err != nil {
							log.Printf("[tls] failed to load regenerated cert: %v", err)
							continue
						}
						certMu.Lock()
						certPtr = &newCert
						certMu.Unlock()
						log.Printf("[tls] certificate hot-swapped with updated IP addresses")
					}
				}
			}()
		}
	}

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	<-c

	fmt.Println("\nShutting down...")
	_ = mgr.Load().DisconnectAll()
	piMgr.GPIO.Close()
	_ = httpServer.Close()
	if tlsServer != nil {
		_ = tlsServer.Close()
	}
}
