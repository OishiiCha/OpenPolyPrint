package maintenance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// Part represents a consumable/spare part for printer maintenance.
type Part struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Category    string  `json:"category"`    // e.g. "Belt", "Nozzle", "Fan", "Filter", "Lubricant"
	PrinterModel string `json:"printerModel"` // e.g. "M5C", "M5", "Universal"
	Stock       int     `json:"stock"`        // quantity on hand
	MinStock    int     `json:"minStock"`     // reorder threshold
	UnitPrice   float64 `json:"unitPrice"`    // current price per unit
	Currency    string  `json:"currency"`     // e.g. "USD", "EUR", "GBP"
	Supplier    string  `json:"supplier"`     // where to buy
	SupplierURL string  `json:"supplierUrl"`  // link to product page
	Notes       string  `json:"notes,omitempty"`
	UpdatedAt   int64   `json:"updatedAt"`
}

// PartStore manages spare parts inventory persisted to disk.
type PartStore struct {
	mu    sync.RWMutex
	file  string
	parts []Part
}

// NewPartStore creates a parts store backed by a JSON file.
func NewPartStore(settingsDir string) *PartStore {
	s := &PartStore{file: filepath.Join(settingsDir, "parts.json")}
	s.load()
	return s
}

func (s *PartStore) load() {
	data, err := os.ReadFile(s.file)
	if err != nil {
		return
	}
	var parts []Part
	if json.Unmarshal(data, &parts) == nil {
		s.parts = parts
	}
}

func (s *PartStore) save() error {
	data, err := json.MarshalIndent(s.parts, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.file, data, 0o600)
}

// List returns all parts sorted by category then name.
func (s *PartStore) List() []Part {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Part, len(s.parts))
	copy(out, s.parts)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Category != out[j].Category {
			return out[i].Category < out[j].Category
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// Add creates a new part.
func (s *PartStore) Add(p Part) Part {
	s.mu.Lock()
	defer s.mu.Unlock()
	p.ID = partID()
	p.UpdatedAt = time.Now().Unix()
	s.parts = append(s.parts, p)
	_ = s.save()
	return p
}

// Update replaces a part by ID.
func (s *PartStore) Update(id string, p Part) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, existing := range s.parts {
		if existing.ID == id {
			p.ID = id
			p.UpdatedAt = time.Now().Unix()
			s.parts[i] = p
			_ = s.save()
			return true
		}
	}
	return false
}

// Remove deletes a part by ID.
func (s *PartStore) Remove(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, p := range s.parts {
		if p.ID == id {
			s.parts = append(s.parts[:i], s.parts[i+1:]...)
			_ = s.save()
			return true
		}
	}
	return false
}

// AdjustStock changes the stock level by delta (can be negative).
func (s *PartStore) AdjustStock(id string, delta int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, p := range s.parts {
		if p.ID == id {
			s.parts[i].Stock += delta
			if s.parts[i].Stock < 0 {
				s.parts[i].Stock = 0
			}
			s.parts[i].UpdatedAt = time.Now().Unix()
			_ = s.save()
			return true
		}
	}
	return false
}

// LowStock returns parts where stock <= minStock.
func (s *PartStore) LowStock() []Part {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []Part
	for _, p := range s.parts {
		if p.Stock <= p.MinStock {
			out = append(out, p)
		}
	}
	return out
}

// TotalValue returns the total inventory value.
func (s *PartStore) TotalValue() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var total float64
	for _, p := range s.parts {
		total += float64(p.Stock) * p.UnitPrice
	}
	return total
}

func partID() string {
	return "part_" + time.Now().Format("20060102150405.000")
}
