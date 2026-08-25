package pi

import (
	"encoding/json"
	"log"
	"net/http"
	"runtime"
)

// ManagerGroup holds the Pi subsystem managers and is used by the HTTP handlers.
type ManagerGroup struct {
	Store   *Store
	GPIO    *Manager
	Sensors *SensorManager
}

// NewManagerGroup creates the Pi store and managers.
func NewManagerGroup(configDir string) *ManagerGroup {
	store := NewStore(configDir)
	gpio := NewManager()
	sensors := NewSensorManager(store)

	// Apply any previously-enabled relay pin on startup
	settings := store.Get()
	if settings.LightRelayEnabled && settings.LightRelayGPIO > 0 && gpio.IsAvailable() {
		_ = gpio.Export(settings.LightRelayGPIO)
		_ = gpio.SetDirection(settings.LightRelayGPIO, "out")
		val := 0
		if settings.LightRelayOn {
			val = 1
		}
		_ = gpio.Write(settings.LightRelayGPIO, val)
	}

	// Start polling if any filament sensors are enabled
	hasSensors := false
	for _, s := range settings.FilamentSensors {
		if s.Enabled && s.GPIOPin > 0 {
			hasSensors = true
			break
		}
	}
	if hasSensors {
		sensors.Start()
	}

	return &ManagerGroup{Store: store, GPIO: gpio, Sensors: sensors}
}

// Mount registers the Pi REST endpoints.
func Mount(mux *http.ServeMux, m *ManagerGroup) {
	mux.HandleFunc("/api/pi", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handlePiSettingsGet(w, r, m)
		case http.MethodPost:
			handlePiSettingsSet(w, r, m)
		default:
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/pi/light", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		handlePiLightToggle(w, r, m)
	})

	mux.HandleFunc("/api/pi/readings", func(w http.ResponseWriter, r *http.Request) {
		HandleReadings(w, r, m)
	})
}

// SensorWithReading combines a sensor config with its last reading.
type SensorWithReading struct {
	ID           int      `json:"id"`
	Enabled      bool     `json:"enabled"`
	Name         string   `json:"name"`
	GPIOPin      int      `json:"gpioPin"`
	FilamentType string   `json:"filamentType"`
	Color        string   `json:"color"`
	Temp         *float64 `json:"temp"`
	Humidity     *float64 `json:"humidity"`
	Error        string   `json:"error,omitempty"`
	UpdatedAt    int64    `json:"updatedAt"`
	HasReading   bool     `json:"hasReading"`
}

func handlePiSettingsGet(w http.ResponseWriter, r *http.Request, m *ManagerGroup) {
	pi := m.Store.Get()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"lightRelayEnabled":    pi.LightRelayEnabled,
		"lightRelayGpio":       pi.LightRelayGPIO,
		"lightRelayOn":         pi.LightRelayOn,
		"filamentSensors":      pi.FilamentSensors,
		"sensorManagerRunning": m.Sensors.IsRunning(),
		"gpioAvailable":        m.GPIO.IsAvailable(),
		"gpioBackend":          m.GPIO.BackendName(),
		"os":                   runtime.GOOS,
	})
}

func handlePiSettingsSet(w http.ResponseWriter, r *http.Request, m *ManagerGroup) {
	var req struct {
		LightRelayEnabled bool             `json:"lightRelayEnabled"`
		LightRelayGPIO    int              `json:"lightRelayGpio"`
		FilamentSensors   []FilamentSensor `json:"filamentSensors"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}

	// Validate filament sensors (max 5)
	if len(req.FilamentSensors) > 5 {
		http.Error(w, `{"error":"maximum 5 filament sensors allowed"}`, http.StatusBadRequest)
		return
	}

	if req.LightRelayEnabled && req.LightRelayGPIO <= 0 {
		http.Error(w, `{"error":"GPIO pin must be > 0"}`, http.StatusBadRequest)
		return
	}

	current := m.Store.Get()

	// If GPIO pin changed, unexport the old one
	if m.GPIO.IsAvailable() && current.LightRelayEnabled && current.LightRelayGPIO > 0 && current.LightRelayGPIO != req.LightRelayGPIO {
		_ = m.GPIO.Unexport(current.LightRelayGPIO)
	}

	// Export and configure the new pin if enabled. GPIO failures are logged
	// but do NOT prevent saving the settings — the user may be configuring
	// pins before hardware is connected, or running in a container where
	// GPIO isn't available yet.
	var gpioWarning string
	if req.LightRelayEnabled && req.LightRelayGPIO > 0 && m.GPIO.IsAvailable() {
		if err := m.GPIO.Export(req.LightRelayGPIO); err != nil {
			gpioWarning = "Failed to export GPIO pin: " + err.Error()
			log.Printf("[pi] warning: %s", gpioWarning)
		} else if err := m.GPIO.SetDirection(req.LightRelayGPIO, "out"); err != nil {
			gpioWarning = "Failed to set GPIO direction: " + err.Error()
			log.Printf("[pi] warning: %s", gpioWarning)
		} else {
			val := 0
			if current.LightRelayOn {
				val = 1
			}
			if err := m.GPIO.Write(req.LightRelayGPIO, val); err != nil {
				gpioWarning = "Failed to write GPIO: " + err.Error()
				log.Printf("[pi] warning: %s", gpioWarning)
			}
		}
	}

	// If disabling, unexport the pin
	if !req.LightRelayEnabled && current.LightRelayEnabled && current.LightRelayGPIO > 0 && m.GPIO.IsAvailable() {
		_ = m.GPIO.Unexport(current.LightRelayGPIO)
	}

	m.Store.Set(Settings{
		LightRelayEnabled: req.LightRelayEnabled,
		LightRelayGPIO:    req.LightRelayGPIO,
		LightRelayOn:      current.LightRelayOn,
		FilamentSensors:   req.FilamentSensors,
	})

	// Restart sensor manager if sensor config changed
	hasSensors := false
	for _, fs := range req.FilamentSensors {
		if fs.Enabled && fs.GPIOPin > 0 {
			hasSensors = true
			break
		}
	}
	if hasSensors {
		m.Sensors.Start()
	} else {
		m.Sensors.Stop()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"message": "Pi settings saved",
		"warning": gpioWarning,
	})
}

func handlePiLightToggle(w http.ResponseWriter, r *http.Request, m *ManagerGroup) {
	var req struct {
		On bool `json:"on"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}

	pi := m.Store.Get()
	if !pi.LightRelayEnabled || pi.LightRelayGPIO <= 0 {
		http.Error(w, `{"error":"light relay not enabled"}`, http.StatusBadRequest)
		return
	}

	if !m.GPIO.IsAvailable() {
		http.Error(w, `{"error":"GPIO not available on this platform"}`, http.StatusBadRequest)
		return
	}

	val := 0
	if req.On {
		val = 1
	}

	if err := m.GPIO.Write(pi.LightRelayGPIO, val); err != nil {
		http.Error(w, `{"error":"Failed to write GPIO: `+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	m.Store.Update(func(s *Settings) {
		s.LightRelayOn = req.On
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"on":      req.On,
	})
}

// HandleReadings returns the latest DHT22 sensor readings combined with sensor
// config and relay state. Exposed as a standalone endpoint.
func HandleReadings(w http.ResponseWriter, r *http.Request, m *ManagerGroup) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	pi := m.Store.Get()
	readings := m.Sensors.GetReadings()

	readingMap := make(map[int]*SensorReading)
	for i := range readings {
		readingMap[readings[i].SensorID] = &readings[i]
	}

	result := make([]SensorWithReading, 0, len(pi.FilamentSensors))
	for _, fs := range pi.FilamentSensors {
		entry := SensorWithReading{
			ID:           fs.ID,
			Enabled:      fs.Enabled,
			Name:         fs.Name,
			GPIOPin:      fs.GPIOPin,
			FilamentType: fs.FilamentType,
			Color:        fs.Color,
		}
		if rd, ok := readingMap[fs.ID]; ok {
			entry.HasReading = true
			entry.UpdatedAt = rd.UpdatedAt
			if rd.Error != "" {
				entry.Error = rd.Error
			} else {
				entry.Temp = &rd.Temp
				entry.Humidity = &rd.Humidity
			}
		}
		result = append(result, entry)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"sensors":              result,
		"count":                len(result),
		"sensorManagerRunning": m.Sensors.IsRunning(),
		"lightRelayEnabled":    pi.LightRelayEnabled,
		"lightRelayGpio":       pi.LightRelayGPIO,
		"lightRelayOn":         pi.LightRelayOn,
		"gpioAvailable":        m.GPIO.IsAvailable(),
		"os":                   runtime.GOOS,
	})
}
