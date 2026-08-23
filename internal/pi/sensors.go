package pi

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

//go:embed all:scripts
var scriptsFS embed.FS

// SensorReading holds the latest reading from a DHT11 sensor.
type SensorReading struct {
	SensorID  int     `json:"sensorId"`
	Temp      float64 `json:"temp"`
	Humidity  float64 `json:"humidity"`
	Error     string  `json:"error,omitempty"`
	UpdatedAt int64   `json:"updatedAt"`
}

// SensorManager periodically reads DHT11 sensors and caches results.
type SensorManager struct {
	mu            sync.RWMutex
	readings      map[int]*SensorReading
	stopCh        chan struct{}
	started       bool
	pythonCmd     string
	pythonChecked bool
	store         *Store
}

// NewSensorManager creates a new sensor manager.
func NewSensorManager(store *Store) *SensorManager {
	return &SensorManager{
		readings: make(map[int]*SensorReading),
		stopCh:   make(chan struct{}),
		store:    store,
	}
}

// Start begins polling sensors every 10 seconds.
func (m *SensorManager) Start() {
	m.mu.Lock()
	if m.started {
		m.mu.Unlock()
		return
	}
	m.started = true
	m.stopCh = make(chan struct{})
	m.mu.Unlock()

	go m.pollLoop()
}

// Stop stops polling.
func (m *SensorManager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.started {
		close(m.stopCh)
		m.started = false
	}
}

// IsRunning reports whether the manager is currently polling.
func (m *SensorManager) IsRunning() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.started
}

// GetReadings returns all current sensor readings.
func (m *SensorManager) GetReadings() []SensorReading {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]SensorReading, 0, len(m.readings))
	for _, r := range m.readings {
		result = append(result, *r)
	}
	return result
}

func (m *SensorManager) pollLoop() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	// Do an immediate read on start
	m.readAll()

	for {
		select {
		case <-ticker.C:
			m.readAll()
		case <-m.stopCh:
			return
		}
	}
}

func (m *SensorManager) readAll() {
	if m.store == nil {
		return
	}
	pi := m.store.Get()
	for _, sensor := range pi.FilamentSensors {
		if !sensor.Enabled || sensor.GPIOPin <= 0 {
			continue
		}
		go m.readSensor(sensor)
	}
}

func (m *SensorManager) readSensor(sensor FilamentSensor) {
	reading := m.readDHT11(sensor.GPIOPin)
	reading.SensorID = sensor.ID
	reading.UpdatedAt = time.Now().Unix()

	m.mu.Lock()
	m.readings[sensor.ID] = &reading
	m.mu.Unlock()
}

// findPython returns the path to the Python executable, or empty string if not found.
func (m *SensorManager) findPython() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.pythonChecked {
		return m.pythonCmd
	}
	m.pythonChecked = true
	// Try PATH lookup first
	for _, name := range []string{"python3", "python"} {
		if path, err := exec.LookPath(name); err == nil {
			m.pythonCmd = path
			return m.pythonCmd
		}
	}
	// Fallback: check common absolute paths (PATH may be minimal in Docker/systemd)
	for _, path := range []string{"/usr/bin/python3", "/usr/local/bin/python3", "/usr/bin/python"} {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			m.pythonCmd = path
			return m.pythonCmd
		}
	}
	return ""
}

// readDHT11 calls the Python script to read a DHT11 sensor on the given pin.
func (m *SensorManager) readDHT11(pin int) SensorReading {
	if runtime.GOOS != "linux" {
		return SensorReading{Error: "DHT11 sensors require Linux (Raspberry Pi)"}
	}

	pythonExe := m.findPython()
	if pythonExe == "" {
		return SensorReading{Error: "Python not installed (install python3 and Adafruit_DHT)"}
	}

	// Extract embedded Python scripts (reader + vendored DHT11 driver) to a
	// temp directory so the reader can import the driver from alongside itself.
	tmpDir, err := os.MkdirTemp("", "dht11_*")
	if err != nil {
		return SensorReading{Error: fmt.Sprintf("create temp dir: %v", err)}
	}
	defer os.RemoveAll(tmpDir)

	for _, name := range []string{"dht11_reader.py", "DHT11.py"} {
		data, err := scriptsFS.ReadFile("scripts/" + name)
		if err != nil {
			return SensorReading{Error: fmt.Sprintf("read embedded script: %v", err)}
		}
		if err := os.WriteFile(filepath.Join(tmpDir, name), data, 0644); err != nil {
			return SensorReading{Error: fmt.Sprintf("write temp script: %v", err)}
		}
	}

	cmd := exec.Command(pythonExe, filepath.Join(tmpDir, "dht11_reader.py"), fmt.Sprintf("%d", pin))

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	if err != nil {
		return SensorReading{Error: "device not detected"}
	}

	var result struct {
		Temp     *float64 `json:"temp"`
		Humidity *float64 `json:"humidity"`
		Error    string   `json:"error"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		return SensorReading{Error: "device not detected"}
	}

	if result.Error != "" {
		return SensorReading{Error: result.Error}
	}

	r := SensorReading{}
	if result.Temp != nil {
		r.Temp = *result.Temp
	}
	if result.Humidity != nil {
		r.Humidity = *result.Humidity
	}
	return r
}
