package queue

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// QueueItem represents a file in the print queue.
type QueueItem struct {
	ID         string `json:"id"`
	PrinterID  string `json:"printerId"`
	Filename   string `json:"filename"`
	AddedAt    int64  `json:"addedAt"`
	Status     string `json:"status"` // "pending", "printing", "done", "failed", "skipped"
	StartedAt  int64  `json:"startedAt,omitempty"`
	FinishedAt int64  `json:"finishedAt,omitempty"`
	Error      string `json:"error,omitempty"`
}

// Store manages the print queue persisted to disk.
type Store struct {
	mu   sync.Mutex
	path string
	items []QueueItem
}

// NewStore creates a queue store backed by a JSON file.
func NewStore(settingsDir string) *Store {
	s := &Store{
		path: filepath.Join(settingsDir, "queue.json"),
	}
	s.load()
	return s
}

func (s *Store) load() {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return
	}
	_ = json.Unmarshal(data, &s.items)
}

func (s *Store) save() {
	data, err := json.MarshalIndent(s.items, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(s.path, data, 0o600)
}

// List returns all queue items sorted by AddedAt.
func (s *Store) List() []QueueItem {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]QueueItem, len(s.items))
	copy(out, s.items)
	sort.Slice(out, func(i, j int) bool { return out[i].AddedAt < out[j].AddedAt })
	return out
}

// Add appends a file to the queue.
func (s *Store) Add(printerID, filename string) QueueItem {
	s.mu.Lock()
	defer s.mu.Unlock()
	item := QueueItem{
		ID:        time.Now().Format("20060102-150405.000"),
		PrinterID: printerID,
		Filename:  filename,
		AddedAt:   time.Now().Unix(),
		Status:    "pending",
	}
	s.items = append(s.items, item)
	s.save()
	return item
}

// Remove deletes a queue item by ID.
func (s *Store) Remove(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, item := range s.items {
		if item.ID == id {
			s.items = append(s.items[:i], s.items[i+1:]...)
			s.save()
			return
		}
	}
}

// Reorder moves a queue item to a new position.
func (s *Store) Reorder(id string, newIndex int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sort.Slice(s.items, func(i, j int) bool { return s.items[i].AddedAt < s.items[j].AddedAt })
	oldIndex := -1
	for i, item := range s.items {
		if item.ID == id {
			oldIndex = i
			break
		}
	}
	if oldIndex == -1 || oldIndex == newIndex {
		return
	}
	item := s.items[oldIndex]
	s.items = append(s.items[:oldIndex], s.items[oldIndex+1:]...)
	if newIndex >= len(s.items) {
		s.items = append(s.items, item)
	} else {
		s.items = append(s.items[:newIndex], append([]QueueItem{item}, s.items[newIndex:]...)...)
	}
	// Reassign AddedAt to maintain order
	for i, it := range s.items {
		s.items[i].AddedAt = it.AddedAt
	}
	s.save()
}

// UpdateStatus updates the status of a queue item.
func (s *Store) UpdateStatus(id, status, errMsg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, item := range s.items {
		if item.ID == id {
			s.items[i].Status = status
			if status == "printing" {
				s.items[i].StartedAt = time.Now().Unix()
			} else if status == "done" || status == "failed" || status == "skipped" {
				s.items[i].FinishedAt = time.Now().Unix()
				if errMsg != "" {
					s.items[i].Error = errMsg
				}
			}
			s.save()
			return
		}
	}
}

// NextPending returns the next pending item for a printer, or nil if none.
func (s *Store) NextPending(printerID string) *QueueItem {
	s.mu.Lock()
	defer s.mu.Unlock()
	sorted := make([]QueueItem, len(s.items))
	copy(sorted, s.items)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].AddedAt < sorted[j].AddedAt })
	for i, item := range sorted {
		if item.PrinterID == printerID && item.Status == "pending" {
			return &sorted[i]
		}
	}
	return nil
}

// Clear removes all completed/failed/skipped items.
func (s *Store) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	var remaining []QueueItem
	for _, item := range s.items {
		if item.Status == "pending" || item.Status == "printing" {
			remaining = append(remaining, item)
		}
	}
	s.items = remaining
	s.save()
}

// ClearAll removes all items.
func (s *Store) ClearAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items = nil
	s.save()
}
