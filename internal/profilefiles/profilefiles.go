package profilefiles

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Category is either "filament" or "print".
type Category string

const (
	CategoryFilament Category = "filament"
	CategoryPrint    Category = "print"
)

// ProfileFile is a stored slicer profile file (e.g. .ini from PrusaSlicer/OrcaSlicer).
type ProfileFile struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`      // display name (user-editable)
	Filename  string   `json:"filename"`  // original filename
	Category  Category `json:"category"`  // "filament" or "print"
	Size      int64    `json:"size"`
	Tags      []string `json:"tags"`
	Slicer    string   `json:"slicer,omitempty"` // e.g. "prusaslicer", "orcaslicer", "cura"
	Notes     string   `json:"notes,omitempty"`
	Content   string   `json:"-"`                // not serialized in list responses
	UploadedAt int64   `json:"uploadedAt"`
}

// Store manages uploaded profile files on disk.
type Store struct {
	mu      sync.RWMutex
	dir     string
	metaFile string
	files   map[string]*ProfileFile // keyed by ID
}

// NewStore creates a profile files store in the given directory.
func NewStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create profilefiles dir: %w", err)
	}
	s := &Store{
		dir:      dir,
		metaFile: filepath.Join(dir, "index.json"),
		files:    make(map[string]*ProfileFile),
	}
	s.load()
	return s, nil
}

func (s *Store) load() {
	data, err := os.ReadFile(s.metaFile)
	if err != nil {
		return
	}
	var list []ProfileFile
	if json.Unmarshal(data, &list) == nil {
		for i := range list {
			s.files[list[i].ID] = &list[i]
		}
	}
}

func (s *Store) save() error {
	list := make([]ProfileFile, 0, len(s.files))
	for _, f := range s.files {
		list = append(list, *f)
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.metaFile, data, 0o600)
}

// List returns all profile files, optionally filtered by category.
func (s *Store) List(category Category) []ProfileFile {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]ProfileFile, 0, len(s.files))
	for _, f := range s.files {
		if category != "" && f.Category != category {
			continue
		}
		out = append(out, *f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Get returns a single profile file by ID (including content).
func (s *Store) Get(id string) (*ProfileFile, error) {
	s.mu.RLock()
	f, ok := s.files[id]
	s.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	result := *f
	// Load content
	path := s.filePath(id)
	data, err := os.ReadFile(path)
	if err != nil {
		return &result, nil // return metadata even if content can't be read
	}
	result.Content = string(data)
	return &result, nil
}

// Add stores a new profile file.
func (s *Store) Add(name, filename string, category Category, content []byte, slicer string, tags []string, notes string) (*ProfileFile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := generateID()
	path := s.filePath(id)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return nil, fmt.Errorf("write file: %w", err)
	}

	pf := &ProfileFile{
		ID:         id,
		Name:       name,
		Filename:   filename,
		Category:   category,
		Size:       int64(len(content)),
		Tags:       tags,
		Slicer:     slicer,
		Notes:      notes,
		UploadedAt: time.Now().Unix(),
	}
	s.files[id] = pf

	if err := s.save(); err != nil {
		_ = os.Remove(path)
		delete(s.files, id)
		return nil, err
	}
	return pf, nil
}

// Update modifies metadata (name, tags, notes, slicer) for a profile file.
func (s *Store) Update(id string, name string, tags []string, notes string, slicer string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, ok := s.files[id]
	if !ok {
		return false
	}
	if name != "" {
		f.Name = name
	}
	f.Tags = tags
	f.Notes = notes
	f.Slicer = slicer
	_ = s.save()
	return true
}

// Remove deletes a profile file and its metadata.
func (s *Store) Remove(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.files[id]
	if !ok {
		return false
	}
	_ = os.Remove(s.filePath(id))
	delete(s.files, id)
	_ = s.save()
	return true
}

// Content returns the raw file content for download.
func (s *Store) Content(id string) ([]byte, string, error) {
	s.mu.RLock()
	f, ok := s.files[id]
	s.mu.RUnlock()
	if !ok {
		return nil, "", fmt.Errorf("not found")
	}
	data, err := os.ReadFile(s.filePath(id))
	if err != nil {
		return nil, "", err
	}
	return data, f.Filename, nil
}

// AllTags returns all unique tags across all files in a category.
func (s *Store) AllTags(category Category) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	seen := map[string]bool{}
	for _, f := range s.files {
		if category != "" && f.Category != category {
			continue
		}
		for _, t := range f.Tags {
			seen[t] = true
		}
	}
	out := make([]string, 0, len(seen))
	for t := range seen {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

func (s *Store) filePath(id string) string {
	return filepath.Join(s.dir, id)
}

func generateID() string {
	return fmt.Sprintf("pf_%d", time.Now().UnixNano())
}

// ParseINI extracts key-value pairs from an INI-style profile file.
// Returns a map of section -> key -> value.
func ParseINI(content string) map[string]map[string]string {
	result := make(map[string]map[string]string)
	var currentSection string
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			currentSection = line[1 : len(line)-1]
			if result[currentSection] == nil {
				result[currentSection] = make(map[string]string)
			}
			continue
		}
		if idx := strings.Index(line, "="); idx > 0 && currentSection != "" {
			key := strings.TrimSpace(line[:idx])
			val := strings.TrimSpace(line[idx+1:])
			result[currentSection][key] = val
		}
	}
	return result
}
