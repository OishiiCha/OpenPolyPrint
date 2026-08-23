package cameras

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

// Store manages camera configs, persisted to a single JSON file.
type Store struct {
	mu        sync.RWMutex
	configDir string
	settings  CameraSettings
}

// NewStore creates a new camera store, loading any existing settings.
func NewStore(configDir string) *Store {
	cs := &Store{
		configDir: configDir,
		settings: CameraSettings{
			Cameras:         []CameraConfig{},
			BuiltinDisabled: map[string]bool{},
			RecSettings:     DefaultRecordingSettings(),
			ActiveCameraIdx: -1,
			CameraLayout:    "single",
		},
	}
	cs.load()
	return cs
}

func (s *Store) settingsPath() string {
	return filepath.Join(s.configDir, "cameras.json")
}

func (s *Store) load() {
	data, err := os.ReadFile(s.settingsPath())
	if err != nil {
		return
	}
	var loaded CameraSettings
	if err := json.Unmarshal(data, &loaded); err != nil {
		return
	}
	s.settings = loaded
	if s.settings.BuiltinDisabled == nil {
		s.settings.BuiltinDisabled = map[string]bool{}
	}
	if s.settings.RecSettings.Format == "" {
		s.settings.RecSettings = DefaultRecordingSettings()
	}
}

func (s *Store) save() error {
	if err := os.MkdirAll(s.configDir, 0o755); err != nil {
		return fmt.Errorf("create camera config dir: %w", err)
	}
	data, err := json.MarshalIndent(s.settings, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal camera settings: %w", err)
	}
	return os.WriteFile(s.settingsPath(), append(data, '\n'), 0o600)
}

// GetSettings returns the current camera settings.
func (s *Store) GetSettings() CameraSettings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.settings
}

// SetSettings replaces the current camera settings.
func (s *Store) SetSettings(settings CameraSettings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.settings = settings
	return s.save()
}

// GetCameras returns the list of configured cameras.
func (s *Store) GetCameras() []CameraConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]CameraConfig{}, s.settings.Cameras...)
}

// AddCamera adds a new camera and persists the settings.
func (s *Store) AddCamera(cam CameraConfig) []CameraConfig {
	s.mu.Lock()
	defer s.mu.Unlock()
	if cam.ID == "" {
		cam.ID = "cam_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	if cam.Name == "" {
		cam.Name = "External Camera"
	}
	if cam.Type == "" {
		cam.Type = "stream"
	}
	cam.Enabled = true
	s.settings.Cameras = append(s.settings.Cameras, cam)
	_ = s.save()
	return append([]CameraConfig{}, s.settings.Cameras...)
}

// UpdateCamera updates an existing camera by ID.
func (s *Store) UpdateCamera(cam CameraConfig) ([]CameraConfig, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, c := range s.settings.Cameras {
		if c.ID == cam.ID {
			s.settings.Cameras[i] = cam
			_ = s.save()
			return append([]CameraConfig{}, s.settings.Cameras...), true
		}
	}
	return nil, false
}

// RemoveCamera removes a camera by ID.
func (s *Store) RemoveCamera(id string) ([]CameraConfig, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, c := range s.settings.Cameras {
		if c.ID == id {
			s.settings.Cameras = append(s.settings.Cameras[:i], s.settings.Cameras[i+1:]...)
			_ = s.save()
			return append([]CameraConfig{}, s.settings.Cameras...), true
		}
	}
	return nil, false
}
