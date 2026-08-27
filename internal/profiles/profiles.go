package profiles

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// Profile is a saved temperature/speed preset for a filament type.
type Profile struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`         // e.g. "PLA Draft", "PETG Quality"
	FilamentType string `json:"filamentType"` // PLA, PETG, ABS, TPU, etc.
	NozzleTemp  float64 `json:"nozzleTemp"`
	BedTemp     float64 `json:"bedTemp"`
	FanSpeed    int     `json:"fanSpeed"`    // 0-100, -1 for default
	PrintSpeed  int     `json:"printSpeed"`  // mm/s or %, 0 for default
	Retraction  float64 `json:"retraction"`  // mm, 0 for default
	Notes       string  `json:"notes,omitempty"`
	CreatedAt   int64   `json:"createdAt"`
}

// Store manages print profiles persisted to disk.
type Store struct {
	mu       sync.RWMutex
	file     string
	profiles []Profile
}

// NewStore creates a profile store backed by a JSON file.
func NewStore(settingsDir string) *Store {
	s := &Store{file: filepath.Join(settingsDir, "profiles.json")}
	s.load()
	return s
}

func (s *Store) load() {
	data, err := os.ReadFile(s.file)
	if err != nil {
		return
	}
	var profiles []Profile
	if json.Unmarshal(data, &profiles) == nil {
		s.profiles = profiles
	}
}

func (s *Store) save() error {
	data, err := json.MarshalIndent(s.profiles, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.file, data, 0o600)
}

// List returns all profiles sorted by name.
func (s *Store) List() []Profile {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Profile, len(s.profiles))
	copy(out, s.profiles)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Add creates a new profile.
func (s *Store) Add(p Profile) Profile {
	s.mu.Lock()
	defer s.mu.Unlock()
	p.ID = profileID()
	if p.CreatedAt == 0 {
		p.CreatedAt = time.Now().Unix()
	}
	s.profiles = append(s.profiles, p)
	_ = s.save()
	return p
}

// Update replaces a profile by ID.
func (s *Store) Update(id string, p Profile) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, existing := range s.profiles {
		if existing.ID == id {
			p.ID = id
			p.CreatedAt = existing.CreatedAt
			s.profiles[i] = p
			_ = s.save()
			return true
		}
	}
	return false
}

// Remove deletes a profile by ID.
func (s *Store) Remove(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, p := range s.profiles {
		if p.ID == id {
			s.profiles = append(s.profiles[:i], s.profiles[i+1:]...)
			_ = s.save()
			return true
		}
	}
	return false
}

func profileID() string {
	return time.Now().Format("20060102150405.000")
}
