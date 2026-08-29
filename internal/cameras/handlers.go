package cameras

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Mount registers the camera HTTP handlers on mux.
func Mount(mux *http.ServeMux, m *Manager) {
	mux.HandleFunc("/api/cameras/settings", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(m.Store().GetSettings())
		case http.MethodPost:
			var s CameraSettings
			if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
				http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
				return
			}
			if err := m.Store().SetSettings(s); err != nil {
				http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "settings": m.Store().GetSettings()})
		default:
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/cameras", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"cameras": m.Store().GetCameras(),
				"count":   len(m.Store().GetCameras()),
			})
		case http.MethodPost:
			var cam CameraConfig
			if err := json.NewDecoder(r.Body).Decode(&cam); err != nil {
				http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
				return
			}
			if cam.Type == "rpicam" {
				for _, existing := range m.Store().GetCameras() {
					if existing.Type == "rpicam" {
						http.Error(w, `{"error":"A Raspberry Pi camera already exists"}`, http.StatusConflict)
						return
					}
				}
			}
			cameras := m.Store().AddCamera(cam)
			if added := last(cameras); added.Enabled && (added.Type == "usb" || added.Type == "rpicam") && added.ID != "" {
				m.Streamers().StartStream(&added)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "cameras": cameras, "count": len(cameras)})
		case http.MethodPut:
			var cam CameraConfig
			if err := json.NewDecoder(r.Body).Decode(&cam); err != nil || cam.ID == "" {
				http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
				return
			}
			m.Streamers().StopStream(cam.ID)
			cameras, ok := m.Store().UpdateCamera(cam)
			if !ok {
				http.Error(w, `{"error":"camera not found"}`, http.StatusNotFound)
				return
			}
			if updated := findByID(cameras, cam.ID); updated.Enabled && (updated.Type == "usb" || updated.Type == "rpicam") && updated.ID != "" {
				m.Streamers().StartStream(&updated)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "cameras": cameras, "count": len(cameras)})
		case http.MethodDelete:
			id := r.URL.Query().Get("id")
			if id == "" {
				http.Error(w, `{"error":"id required"}`, http.StatusBadRequest)
				return
			}
			m.Streamers().StopStream(id)
			cameras, ok := m.Store().RemoveCamera(id)
			if !ok {
				http.Error(w, `{"error":"camera not found"}`, http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "cameras": cameras, "count": len(cameras)})
		default:
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/cameras/status", func(w http.ResponseWriter, r *http.Request) {
		status := map[string]CameraStreamStatus{}
		for _, cam := range m.Store().GetCameras() {
			if cam.Type != "usb" && cam.Type != "rpicam" {
				continue
			}
			if streamer := m.Streamers().GetStream(cam.ID); streamer != nil {
				status[cam.ID] = streamer.Status()
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": status})
	})

	mux.HandleFunc("/api/cameras/usb/list", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		devices := listUsbCameras()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"devices": devices, "count": len(devices)})
	})

	mux.HandleFunc("/api/cameras/mipi/list", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		devices := listMipiCameras()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"devices": devices, "count": len(devices)})
	})

	mux.HandleFunc("/api/cameras/usb/preview", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		deviceID := r.URL.Query().Get("deviceId")
		deviceLabel := r.URL.Query().Get("deviceLabel")
		if deviceID == "" {
			http.Error(w, `{"error":"deviceId required"}`, http.StatusBadRequest)
			return
		}

		var previewID string
		var previewCfg *CameraConfig
		for _, cam := range m.Store().GetCameras() {
			if cam.Type == "usb" && (cam.DeviceID == deviceID || cam.DeviceLabel == deviceLabel) {
				previewID = cam.ID
				previewCfg = &cam
				if streamer := m.Streamers().GetStream(cam.ID); streamer != nil && streamer.IsRunning() {
					serveMjpeg(w, r, streamer)
					return
				}
				break
			}
		}

		if previewID == "" {
			previewID = "preview-" + deviceID
		}

		streamer := m.Streamers().GetStream(previewID)
		if streamer == nil {
			cam := CameraConfig{
				ID:          previewID,
				Name:        "Preview: " + deviceLabel,
				Type:        "usb",
				DeviceID:    deviceID,
				DeviceLabel: deviceLabel,
				Brightness:  0,
				Flip:        "",
			}
			if previewCfg != nil {
				cam.Brightness = previewCfg.Brightness
				cam.Flip = previewCfg.Flip
			}
			m.Streamers().StartStream(&cam)
			time.Sleep(1500 * time.Millisecond)
			streamer = m.Streamers().GetStream(previewID)
		}
		if streamer == nil {
			http.Error(w, `{"error":"failed to start preview"}`, http.StatusInternalServerError)
			return
		}
		serveMjpeg(w, r, streamer)
		m.Streamers().StopStream(previewID)
	})

	mux.HandleFunc("/api/cameras/mipi/preview", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		deviceID := r.URL.Query().Get("deviceId")
		deviceLabel := r.URL.Query().Get("deviceLabel")
		sensor := r.URL.Query().Get("sensor")
		if deviceID == "" && sensor == "" {
			http.Error(w, `{"error":"deviceId or sensor required"}`, http.StatusBadRequest)
			return
		}

		var previewID string
		var previewCfg *CameraConfig
		for _, cam := range m.Store().GetCameras() {
			if cam.Type == "rpicam" && (cam.DeviceID == deviceID || cam.Sensor == sensor) {
				previewID = cam.ID
				previewCfg = &cam
				if streamer := m.Streamers().GetStream(cam.ID); streamer != nil && streamer.IsRunning() {
					serveMjpeg(w, r, streamer)
					return
				}
				break
			}
		}

		if previewID == "" {
			previewID = "preview-mipi-" + deviceID
			if previewID == "preview-mipi-" {
				previewID = "preview-mipi-" + sensor
			}
		}

		streamer := m.Streamers().GetStream(previewID)
		if streamer == nil {
			cam := CameraConfig{
				ID:          previewID,
				Name:        "Preview: " + deviceLabel,
				Type:        "rpicam",
				DeviceID:    deviceID,
				DeviceLabel: deviceLabel,
				Sensor:      sensor,
				Brightness:  0,
				Flip:        "",
			}
			if previewCfg != nil {
				cam.Brightness = previewCfg.Brightness
				cam.Flip = previewCfg.Flip
			}
			m.Streamers().StartStream(&cam)
			time.Sleep(1500 * time.Millisecond)
			streamer = m.Streamers().GetStream(previewID)
		}
		if streamer == nil {
			http.Error(w, `{"error":"failed to start preview"}`, http.StatusInternalServerError)
			return
		}
		serveMjpeg(w, r, streamer)
		m.Streamers().StopStream(previewID)
	})

	// Dedicated stream endpoint for configured cameras.
	// Unlike the preview endpoints, this does NOT sleep waiting for the first
	// frame and does NOT stop the stream when the client disconnects. The
	// streamer is expected to already be running (started at app startup or
	// when the camera was added). If it's not running, we start it and wait
	// briefly for the first frame.
	mux.HandleFunc("/api/cameras/{id}/stream", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id == "" {
			http.Error(w, `{"error":"id required"}`, http.StatusBadRequest)
			return
		}

		streamer := m.Streamers().GetStream(id)
		if streamer == nil || !streamer.IsRunning() {
			// Stream not running — try to start it from saved config
			cfg := findByID(m.Store().GetCameras(), id)
			if cfg.ID == "" {
				http.Error(w, `{"error":"camera not found"}`, http.StatusNotFound)
				return
			}
			m.Streamers().StartStream(&cfg)
			// Wait for first frame (shorter than preview — stream should be warm)
			for i := 0; i < 30; i++ {
				streamer = m.Streamers().GetStream(id)
				if streamer != nil && streamer.IsRunning() {
					if s := streamer.Status(); s.Frames > 0 {
						break
					}
				}
				time.Sleep(100 * time.Millisecond)
			}
		}

		if streamer == nil {
			http.Error(w, `{"error":"failed to start stream"}`, http.StatusInternalServerError)
			return
		}
		serveMjpeg(w, r, streamer)
		// Do NOT stop the stream on disconnect — it stays running for other
		// clients and instant reconnection.
	})

	mux.HandleFunc("/api/cameras/{id}/record/start", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		id := r.PathValue("id")
		if id == "" {
			http.Error(w, `{"error":"id required"}`, http.StatusBadRequest)
			return
		}
		streamer := m.Streamers().GetStream(id)
		if streamer == nil {
			cfg := findByID(m.Store().GetCameras(), id)
			if cfg.ID == "" {
				http.Error(w, `{"error":"camera not found"}`, http.StatusNotFound)
				return
			}
			m.Streamers().StartStream(&cfg)
			time.Sleep(1500 * time.Millisecond)
			streamer = m.Streamers().GetStream(id)
		}

		var req struct {
			Printer string `json:"printer"`
			Gcode   string `json:"gcode"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)

		filename := buildRecordFilename(req.Printer, req.Gcode)
		path, err := m.Records().Start(id, streamer, filename)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "path": path})
	})

	mux.HandleFunc("/api/cameras/{id}/record/stop", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		id := r.PathValue("id")
		path, err := m.Records().Stop(id)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "path": path})
	})

	mux.HandleFunc("/api/cameras/{id}/record/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		id := r.PathValue("id")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"video":     m.Records().Status(id),
			"timelapse": m.Timelapses().Status(id),
		})
	})

	mux.HandleFunc("/api/cameras/{id}/record/timelapse/start", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		id := r.PathValue("id")
		if id == "" {
			http.Error(w, `{"error":"id required"}`, http.StatusBadRequest)
			return
		}
		streamer := m.Streamers().GetStream(id)
		if streamer == nil {
			cfg := findByID(m.Store().GetCameras(), id)
			if cfg.ID == "" {
				http.Error(w, `{"error":"camera not found"}`, http.StatusNotFound)
				return
			}
			m.Streamers().StartStream(&cfg)
			time.Sleep(1500 * time.Millisecond)
			streamer = m.Streamers().GetStream(id)
		}

		var req struct {
			Printer  string  `json:"printer"`
			Gcode    string  `json:"gcode"`
			Interval float64 `json:"intervalSeconds"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Interval <= 0 {
			req.Interval = 1
		}

		filename := buildRecordFilename(req.Printer, req.Gcode)
		path, err := m.Timelapses().Start(id, streamer, filename, req.Interval)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "path": path})
	})

	mux.HandleFunc("/api/cameras/{id}/record/timelapse/stop", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		id := r.PathValue("id")
		path, err := m.Timelapses().Stop(id)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "path": path})
	})

	mux.HandleFunc("/api/cameras/{id}/record/timelapse/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		id := r.PathValue("id")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"timelapse": m.Timelapses().IsRecording(id)})
	})

	mux.HandleFunc("/api/recordings", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		files, err := ListRecordings()
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"recordings": files, "count": len(files)})
	})

	mux.HandleFunc("/api/recordings/videos", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		files, err := ListVideos()
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"recordings": files, "count": len(files)})
	})

	mux.HandleFunc("/api/recordings/timelapses", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		files, err := ListTimelapses()
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"recordings": files, "count": len(files)})
	})

	mux.HandleFunc("/api/recordings/names", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"names": m.Names().GetAll()})
	})

	mux.HandleFunc("/api/recordings/{folder}/{filename}/thumb", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		folder := r.PathValue("folder")
		filename := r.PathValue("filename")
		data, contentType, err := ServeThumb(folder, filename)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Cache-Control", "public, max-age=300")
		_, _ = w.Write(data)
	})

	mux.HandleFunc("/api/recordings/{folder}/{filename}/convert/timelapse", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		folder := r.PathValue("folder")
		filename := r.PathValue("filename")
		path, err := ConvertToTimelapse(folder, filename)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "path": path})
	})

	mux.HandleFunc("/api/recordings/{folder}/{filename}/name", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		folder := r.PathValue("folder")
		filename := r.PathValue("filename")
		if folder == "" || filename == "" {
			http.Error(w, `{"error":"folder and filename required"}`, http.StatusBadRequest)
			return
		}
		var req struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
			return
		}
		m.Names().Set(folder, filename, req.Name)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "name": m.Names().Get(folder, filename)})
	})

	mux.HandleFunc("/api/recordings/{folder}/{filename}/delete", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		folder := r.PathValue("folder")
		filename := r.PathValue("filename")
		if folder == "" || filename == "" {
			http.Error(w, `{"error":"folder and filename required"}`, http.StatusBadRequest)
			return
		}
		path := filepath.Join("recordings", folder, filename)
		if _, err := os.Stat(path); err != nil {
			http.Error(w, `{"error":"file not found"}`, http.StatusNotFound)
			return
		}
		if err := os.Remove(path); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
			return
		}
		m.Names().Set(folder, filename, "")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
	})

	mux.Handle("/recordings/", http.StripPrefix("/recordings/", http.FileServer(http.Dir("recordings"))))
}

func buildRecordFilename(printer, gcode string) string {
	clean := func(s string) string {
		s = strings.ToLower(strings.TrimSpace(s))
		s = strings.ReplaceAll(s, " ", "_")
		s = strings.ReplaceAll(s, ".gcode", "")
		var b strings.Builder
		for _, r := range s {
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
				b.WriteRune(r)
			}
		}
		return b.String()
	}

	parts := []string{}
	if p := clean(printer); p != "" {
		parts = append(parts, p)
	}
	if g := clean(gcode); g != "" {
		parts = append(parts, g)
	}
	parts = append(parts, time.Now().Format("20060102_15-04"))
	return strings.Join(parts, "_") + ".mkv"
}

type usbDevice struct {
	Name        string `json:"name"`
	DeviceID    string `json:"deviceId"`
	DeviceLabel string `json:"deviceLabel"`
}

type mipiDevice struct {
	Index  string `json:"index"`
	Sensor string `json:"sensor"`
	Name   string `json:"name"`
}

func listUsbCameras() []usbDevice {
	var devices []usbDevice
	switch runtime.GOOS {
	case "windows":
		cmd := exec.Command("ffmpeg", "-list_devices", "true", "-f", "dshow", "-i", "dummy")
		output, _ := cmd.CombinedOutput()
		for _, line := range strings.Split(string(output), "\n") {
			if strings.Contains(line, "(video)") {
				name := extractQuoted(line)
				if name != "" {
					devices = append(devices, usbDevice{Name: name, DeviceID: name, DeviceLabel: name})
				}
			}
		}
	case "linux":
		matches, _ := filepath.Glob("/dev/video*")
		for _, m := range matches {
			devName := filepath.Base(m)
			sysPath := filepath.Join("/sys/class/video4linux", devName)
			if _, err := os.Stat(sysPath); err != nil {
				continue
			}
			deviceLink := filepath.Join(sysPath, "device")
			realDevicePath, err := filepath.EvalSymlinks(deviceLink)
			if err != nil {
				continue
			}
			if !strings.Contains(realDevicePath, "/usb") {
				continue
			}
			cardName := m
			if nameBytes, err := os.ReadFile(filepath.Join(sysPath, "name")); err == nil {
				cardName = strings.TrimSpace(string(nameBytes))
			}
			label := cardName
			vendorBytes, _ := os.ReadFile(filepath.Join(realDevicePath, "idVendor"))
			productBytes, _ := os.ReadFile(filepath.Join(realDevicePath, "idProduct"))
			vendor := strings.TrimSpace(string(vendorBytes))
			product := strings.TrimSpace(string(productBytes))
			if vendor != "" && product != "" {
				label = fmt.Sprintf("%s (%s:%s)", cardName, vendor, product)
			}
			devices = append(devices, usbDevice{Name: cardName, DeviceID: m, DeviceLabel: label})
		}
	case "darwin":
		cmd := exec.Command("ffmpeg", "-list_devices", "true", "-f", "avfoundation", "-i", "dummy")
		output, _ := cmd.CombinedOutput()
		for _, line := range strings.Split(string(output), "\n") {
			if strings.Contains(line, "(video)") {
				name := extractQuoted(line)
				if name != "" {
					idx := extractBracketNum(line)
					devices = append(devices, usbDevice{Name: name, DeviceID: idx, DeviceLabel: name})
				}
			}
		}
	}
	return devices
}

func listMipiCameras() []mipiDevice {
	if runtime.GOOS != "linux" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "rpicam-hello", "--list-cameras").CombinedOutput()
	if err != nil {
		return nil
	}
	var devices []mipiDevice
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) >= 3 && fields[1] == ":" {
			devices = append(devices, mipiDevice{
				Index:  fields[0],
				Sensor: fields[2],
				Name:   strings.Join(fields[2:], " "),
			})
		}
	}
	return devices
}

func serveMjpeg(w http.ResponseWriter, r *http.Request, streamer *UsbCameraStreamer) {
	w.Header().Set("Content-Type", "multipart/x-mixed-replace; boundary=frame")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Connection", "close")
	sub := streamer.Subscribe()
	defer streamer.Unsubscribe(sub)
	for {
		select {
		case frame, ok := <-sub:
			if !ok {
				return
			}
			_, _ = fmt.Fprintf(w, "--frame\r\nContent-Type: image/jpeg\r\nContent-Length: %d\r\n\r\n", len(frame))
			_, _ = w.Write(frame)
			_, _ = w.Write([]byte("\r\n"))
			if fl, ok := w.(http.Flusher); ok {
				fl.Flush()
			}
		case <-r.Context().Done():
			return
		}
	}
}

func extractQuoted(s string) string {
	start := strings.Index(s, "\"")
	if start < 0 {
		return ""
	}
	end := strings.Index(s[start+1:], "\"")
	if end < 0 {
		return ""
	}
	return s[start+1 : start+1+end]
}

func extractBracketNum(s string) string {
	start := strings.Index(s, "[")
	if start < 0 {
		return ""
	}
	end := strings.Index(s[start:], "]")
	if end < 0 {
		return ""
	}
	return strings.TrimSpace(s[start+1 : start+end])
}

func last(cameras []CameraConfig) CameraConfig {
	if len(cameras) == 0 {
		return CameraConfig{}
	}
	return cameras[len(cameras)-1]
}

func findByID(cameras []CameraConfig, id string) CameraConfig {
	for _, c := range cameras {
		if c.ID == id {
			return c
		}
	}
	return CameraConfig{}
}
