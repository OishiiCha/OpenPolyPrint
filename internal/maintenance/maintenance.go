package maintenance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// Reminder tracks a maintenance task for a printer.
type Reminder struct {
	ID            string  `json:"id"`
	PrinterID     string  `json:"printerId"`
	PrinterName   string  `json:"printerName"`
	Task          string  `json:"task"`          // e.g. "Lubricate rods", "Check belt tension"
	IntervalHours float64 `json:"intervalHours"` // how often (in print hours)
	LastPerformed int64   `json:"lastPerformed"`  // unix timestamp
	Notes         string  `json:"notes,omitempty"`
}

// Status shows whether maintenance is due.
type Status struct {
	Reminder
	HoursSince   float64 `json:"hoursSince"`
	HoursUntilDue float64 `json:"hoursUntilDue"` // negative = overdue
	IsDue        bool    `json:"isDue"`
}

// Store manages maintenance reminders persisted to disk.
type Store struct {
	mu        sync.RWMutex
	file      string
	reminders []Reminder
}

// NewStore creates a maintenance store backed by a JSON file.
func NewStore(settingsDir string) *Store {
	s := &Store{file: filepath.Join(settingsDir, "maintenance.json")}
	s.load()
	return s
}

func (s *Store) load() {
	data, err := os.ReadFile(s.file)
	if err != nil {
		return
	}
	var reminders []Reminder
	if json.Unmarshal(data, &reminders) == nil {
		s.reminders = reminders
	}
}

func (s *Store) save() error {
	data, err := json.MarshalIndent(s.reminders, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.file, data, 0o600)
}

// List returns all reminders.
func (s *Store) List() []Reminder {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Reminder, len(s.reminders))
	copy(out, s.reminders)
	return out
}

// Add creates a new reminder.
func (s *Store) Add(r Reminder) Reminder {
	s.mu.Lock()
	defer s.mu.Unlock()
	r.ID = reminderID()
	if r.LastPerformed == 0 {
		r.LastPerformed = time.Now().Unix()
	}
	s.reminders = append(s.reminders, r)
	_ = s.save()
	return r
}

// Update replaces a reminder by ID.
func (s *Store) Update(id string, r Reminder) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, existing := range s.reminders {
		if existing.ID == id {
			r.ID = id
			s.reminders[i] = r
			_ = s.save()
			return true
		}
	}
	return false
}

// Remove deletes a reminder by ID.
func (s *Store) Remove(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, r := range s.reminders {
		if r.ID == id {
			s.reminders = append(s.reminders[:i], s.reminders[i+1:]...)
			_ = s.save()
			return true
		}
	}
	return false
}

// MarkPerformed updates the lastPerformed timestamp for a reminder.
func (s *Store) MarkPerformed(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, r := range s.reminders {
		if r.ID == id {
			s.reminders[i].LastPerformed = time.Now().Unix()
			_ = s.save()
			return true
		}
	}
	return false
}

// Statuses returns maintenance status for all reminders, given a map of
// printerID -> total print hours.
func (s *Store) Statuses(printHours map[string]float64) []Status {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Status, len(s.reminders))
	for i, r := range s.reminders {
		hours := printHours[r.PrinterID]
		// Estimate hours since last performed
		lastPerformedTime := time.Unix(r.LastPerformed, 0)
		daysSince := time.Since(lastPerformedTime).Hours() / 24
		// Use actual print hours if available, otherwise estimate from time elapsed
		// (assume 2 hours/day average if no data)
		hoursSince := hours
		if hours == 0 {
			hoursSince = daysSince * 2 // rough estimate
		}
		untilDue := r.IntervalHours - hoursSince
		out[i] = Status{
			Reminder:      r,
			HoursSince:    hoursSince,
			HoursUntilDue: untilDue,
			IsDue:         untilDue <= 0,
		}
	}
	sort.Slice(out, func(i, j int) bool {
		// Overdue first, then by hours until due
		if out[i].IsDue != out[j].IsDue {
			return out[i].IsDue
		}
		return out[i].HoursUntilDue < out[j].HoursUntilDue
	})
	return out
}

func reminderID() string {
	return time.Now().Format("20060102150405.000")
}
