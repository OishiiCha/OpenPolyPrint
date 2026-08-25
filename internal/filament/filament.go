package filament

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// Spool represents a filament spool in inventory.
type Spool struct {
	ID         string  `json:"id"`
	Brand      string  `json:"brand"`
	Type       string  `json:"type"`       // PLA, PETG, ABS, TPU, etc.
	Color      string  `json:"color"`      // hex color
	ColorName  string  `json:"colorName"`  // human-readable color name
	WeightG    float64 `json:"weightG"`    // total weight in grams
	RemainingG float64 `json:"remainingG"` // remaining weight in grams
	Diameter   float64 `json:"diameter"`   // 1.75 or 2.85
	Cost       float64 `json:"cost"`       // cost per spool
	AddedAt    int64   `json:"addedAt"`
}

// Store manages filament inventory persisted to disk.
type Store struct {
	mu    sync.Mutex
	path  string
	spools []Spool
}

// NewStore creates a filament store backed by a JSON file.
func NewStore(settingsDir string) *Store {
	s := &Store{
		path: filepath.Join(settingsDir, "filament.json"),
	}
	s.load()
	return s
}

func (s *Store) load() {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return
	}
	_ = json.Unmarshal(data, &s.spools)
}

func (s *Store) save() {
	data, err := json.MarshalIndent(s.spools, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(s.path, data, 0o600)
}

// List returns all spools sorted by AddedAt.
func (s *Store) List() []Spool {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Spool, len(s.spools))
	copy(out, s.spools)
	sort.Slice(out, func(i, j int) bool { return out[i].AddedAt < out[j].AddedAt })
	return out
}

// Add creates a new spool.
func (s *Store) Add(spool Spool) Spool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if spool.ID == "" {
		spool.ID = time.Now().Format("20060102-150405.000")
	}
	spool.AddedAt = time.Now().Unix()
	if spool.RemainingG == 0 {
		spool.RemainingG = spool.WeightG
	}
	s.spools = append(s.spools, spool)
	s.save()
	return spool
}

// Update modifies an existing spool.
func (s *Store) Update(id string, spool Spool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, existing := range s.spools {
		if existing.ID == id {
			spool.ID = id
			spool.AddedAt = existing.AddedAt
			s.spools[i] = spool
			s.save()
			return true
		}
	}
	return false
}

// Remove deletes a spool by ID.
func (s *Store) Remove(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, spool := range s.spools {
		if spool.ID == id {
			s.spools = append(s.spools[:i], s.spools[i+1:]...)
			s.save()
			return true
		}
	}
	return false
}

// UseFilament reduces the remaining amount on a spool.
func (s *Store) UseFilament(id string, grams float64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, spool := range s.spools {
		if spool.ID == id {
			s.spools[i].RemainingG -= grams
			if s.spools[i].RemainingG < 0 {
				s.spools[i].RemainingG = 0
			}
			s.save()
			return true
		}
	}
	return false
}
