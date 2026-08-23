package pi

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sync"
)

// Store persists Raspberry Pi settings to disk.
type Store struct {
	mu       sync.Mutex
	settings Settings
	file     string
}

// NewStore creates a Store and loads existing settings from the config directory.
func NewStore(configDir string) *Store {
	file := filepath.Join(configDir, "pi.json")
	s := &Store{file: file, settings: DefaultSettings()}
	data, err := os.ReadFile(file)
	if err == nil {
		_ = json.Unmarshal(data, &s.settings)
	} else {
		_ = s.Save()
	}
	return s
}

// Get returns the current settings.
func (s *Store) Get() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.settings
}

// Set replaces and saves the settings.
func (s *Store) Set(settings Settings) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.settings = settings
	s.saveLocked()
}

// Update applies a mutation and saves.
func (s *Store) Update(fn func(*Settings)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn(&s.settings)
	s.saveLocked()
}

// Save writes the current settings to disk.
func (s *Store) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveLocked()
}

func (s *Store) saveLocked() error {
	if s.file == "" {
		return nil
	}
	data, err := json.MarshalIndent(s.settings, "", "  ")
	if err != nil {
		log.Printf("pi: marshal settings error: %v", err)
		return err
	}
	data = append(data, '\n')
	if err := os.WriteFile(s.file, data, 0o600); err != nil {
		log.Printf("pi: write settings error: %v", err)
		return err
	}
	return nil
}
