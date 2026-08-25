package tempstore

import (
	"sync"
	"time"
)

// Sample is a single temperature reading.
type Sample struct {
	Time         int64   `json:"time"`
	Nozzle       float64 `json:"nozzle"`
	TargetNozzle float64 `json:"targetNozzle"`
	Bed          float64 `json:"bed"`
	TargetBed    float64 `json:"targetBed"`
}

// Store keeps a ring buffer of temperature samples per printer.
type Store struct {
	mu      sync.RWMutex
	samples map[string][]Sample // printerID → samples (newest at end)
	max     int
}

// New creates a Store that keeps up to max samples per printer.
func New(max int) *Store {
	if max <= 0 {
		max = 600 // 10 min at 1s, 50 min at 12s
	}
	return &Store{
		samples: make(map[string][]Sample),
		max:     max,
	}
}

// Record adds a temperature sample for a printer.
func (s *Store) Record(printerID string, nozzle, targetNozzle, bed, targetBed float64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sample := Sample{
		Time:         time.Now().Unix(),
		Nozzle:       nozzle,
		TargetNozzle: targetNozzle,
		Bed:          bed,
		TargetBed:    targetBed,
	}

	buf := s.samples[printerID]
	buf = append(buf, sample)
	if len(buf) > s.max {
		buf = buf[len(buf)-s.max:]
	}
	s.samples[printerID] = buf
}

// Get returns the samples for a printer.
func (s *Store) Get(printerID string) []Sample {
	s.mu.RLock()
	defer s.mu.RUnlock()
	buf := s.samples[printerID]
	out := make([]Sample, len(buf))
	copy(out, buf)
	return out
}

// GetAll returns samples for all printers.
func (s *Store) GetAll() map[string][]Sample {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string][]Sample, len(s.samples))
	for k, v := range s.samples {
		cp := make([]Sample, len(v))
		copy(cp, v)
		out[k] = cp
	}
	return out
}

// Clear removes samples for a printer.
func (s *Store) Clear(printerID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.samples, printerID)
}
