package history

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"time"
)

// Record is a single finished print entry.
type Record struct {
	ID        string    `json:"id"`
	Printer   string    `json:"printer"`
	File      string    `json:"file"`
	Started   string    `json:"started"`
	Duration  string    `json:"duration"`
	Result    string    `json:"result"`
	StartedAt time.Time `json:"started_at"`
	EndedAt   time.Time `json:"ended_at"`
}

// Store persists print history to a JSON file.
type Store struct {
	mu      sync.RWMutex
	file    string
	records []Record
	nextID  int
}

// NewStore creates or loads the history store.
func NewStore(dir string) *Store {
	s := &Store{file: filepath.Join(dir, "history.json")}
	_ = os.MkdirAll(dir, 0o755)
	if data, err := os.ReadFile(s.file); err == nil {
		var persisted struct {
			Records []Record `json:"records"`
			NextID  int      `json:"nextId"`
		}
		if json.Unmarshal(data, &persisted) == nil {
			s.records = persisted.Records
			s.nextID = persisted.NextID
		}
	}
	if s.nextID == 0 {
		s.nextID = 1
	}
	return s
}

// List returns all history records, newest first.
func (s *Store) List() []Record {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Record, len(s.records))
	copy(out, s.records)
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt.After(out[j].StartedAt) })
	return out
}

// Add records a finished print.
func (s *Store) Add(printer, file, result string, startedAt, endedAt time.Time) Record {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := Record{
		ID:        strconv.Itoa(s.nextID),
		Printer:   printer,
		File:      file,
		Started:   startedAt.Format("2006-01-02 15:04"),
		Duration:  formatDuration(startedAt, endedAt),
		Result:    result,
		StartedAt: startedAt,
		EndedAt:   endedAt,
	}
	s.nextID++
	s.records = append(s.records, r)
	_ = s.saveLocked()
	return r
}

// Delete removes a record by id.
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, r := range s.records {
		if r.ID == id {
			s.records = append(s.records[:i], s.records[i+1:]...)
			return s.saveLocked()
		}
	}
	return fmt.Errorf("record not found")
}

// Clear removes all records.
func (s *Store) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = nil
	return s.saveLocked()
}

func (s *Store) saveLocked() error {
	data, err := json.MarshalIndent(struct {
		Records []Record `json:"records"`
		NextID  int      `json:"nextId"`
	}{Records: s.records, NextID: s.nextID}, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(s.file, data, 0o600)
}

func formatDuration(a, b time.Time) string {
	d := b.Sub(a)
	if d < 0 {
		d = 0
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	sec := int(d.Seconds()) % 60
	return fmt.Sprintf("%02d:%02d:%02d", h, m, sec)
}
