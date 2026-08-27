package ai

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// PromptPreset is a saved prompt that can be loaded by name.
type PromptPreset struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Prompt    string    `json:"prompt"`
	CreatedAt time.Time `json:"createdAt"`
}

// PromptStore manages saved prompt presets in a JSON file.
type PromptStore struct {
	mu   sync.Mutex
	path string
}

// NewPromptStore creates a prompt store backed by the given file path.
func NewPromptStore(path string) *PromptStore {
	return &PromptStore{path: path}
}

func (s *PromptStore) load() ([]PromptPreset, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return []PromptPreset{}, nil
		}
		return nil, err
	}
	var presets []PromptPreset
	if err := json.Unmarshal(data, &presets); err != nil {
		return nil, err
	}
	return presets, nil
}

func (s *PromptStore) save(presets []PromptPreset) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(presets, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, append(data, '\n'), 0o644)
}

// List returns all saved prompt presets sorted by name.
func (s *PromptStore) List() ([]PromptPreset, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	presets, err := s.load()
	if err != nil {
		return nil, err
	}
	sort.Slice(presets, func(i, j int) bool { return presets[i].Name < presets[j].Name })
	return presets, nil
}

// Save adds or updates a prompt preset. If id is empty, a new one is created.
func (s *PromptStore) Save(id, name, prompt string) (*PromptPreset, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	presets, err := s.load()
	if err != nil {
		return nil, err
	}

	if id != "" {
		for i, p := range presets {
			if p.ID == id {
				presets[i].Name = name
				presets[i].Prompt = prompt
				if err := s.save(presets); err != nil {
					return nil, err
				}
				return &presets[i], nil
			}
		}
	}

	// Create new
	newPreset := PromptPreset{
		ID:        fmt.Sprintf("prompt_%d", time.Now().UnixNano()),
		Name:      name,
		Prompt:    prompt,
		CreatedAt: time.Now(),
	}
	presets = append(presets, newPreset)
	if err := s.save(presets); err != nil {
		return nil, err
	}
	return &newPreset, nil
}

// Delete removes a prompt preset by ID.
func (s *PromptStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	presets, err := s.load()
	if err != nil {
		return err
	}
	for i, p := range presets {
		if p.ID == id {
			presets = append(presets[:i], presets[i+1:]...)
			return s.save(presets)
		}
	}
	return fmt.Errorf("prompt not found")
}
