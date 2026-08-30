package stlfiles

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// STLFile is a stored STL (or OBJ) 3D model file.
type STLFile struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`     // display name (user-editable)
	Filename   string   `json:"filename"` // original filename
	Size       int64    `json:"size"`
	Tags       []string `json:"tags"`
	Notes      string   `json:"notes,omitempty"`
	UploadedAt int64    `json:"uploadedAt"`
}

// Store manages uploaded STL/3D model files on disk.
type Store struct {
	mu       sync.RWMutex
	dir      string
	metaFile string
	files    map[string]*STLFile
}

// NewStore creates an STL files store in the given directory.
func NewStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create stlfiles dir: %w", err)
	}
	s := &Store{
		dir:      dir,
		metaFile: filepath.Join(dir, "index.json"),
		files:    make(map[string]*STLFile),
	}
	s.load()
	return s, nil
}

func (s *Store) load() {
	data, err := os.ReadFile(s.metaFile)
	if err != nil {
		return
	}
	var list []STLFile
	if json.Unmarshal(data, &list) == nil {
		for i := range list {
			s.files[list[i].ID] = &list[i]
		}
	}
}

func (s *Store) save() error {
	list := make([]STLFile, 0, len(s.files))
	for _, f := range s.files {
		list = append(list, *f)
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.metaFile, data, 0o600)
}

// List returns all STL files sorted by name.
func (s *Store) List() []STLFile {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]STLFile, 0, len(s.files))
	for _, f := range s.files {
		out = append(out, *f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Get returns a single STL file by ID.
func (s *Store) Get(id string) (*STLFile, error) {
	s.mu.RLock()
	f, ok := s.files[id]
	s.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	result := *f
	return &result, nil
}

// Add stores a new STL file.
func (s *Store) Add(name, filename string, content []byte, tags []string, notes string) (*STLFile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := generateID()
	path := s.filePath(id)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return nil, fmt.Errorf("write file: %w", err)
	}

	f := &STLFile{
		ID:         id,
		Name:       name,
		Filename:   filename,
		Size:       int64(len(content)),
		Tags:       tags,
		Notes:      notes,
		UploadedAt: time.Now().Unix(),
	}
	s.files[id] = f

	if err := s.save(); err != nil {
		_ = os.Remove(path)
		delete(s.files, id)
		return nil, err
	}
	return f, nil
}

// Update modifies metadata (name, filename, tags, notes) for an STL file.
func (s *Store) Update(id string, name string, filename string, tags []string, notes string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, ok := s.files[id]
	if !ok {
		return false
	}
	if name != "" {
		f.Name = name
	}
	if filename != "" {
		f.Filename = filename
	}
	f.Tags = tags
	f.Notes = notes
	_ = s.save()
	return true
}

// Remove deletes an STL file and its metadata.
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

// AllTags returns all unique tags across all files.
func (s *Store) AllTags() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	seen := map[string]bool{}
	for _, f := range s.files {
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
	return fmt.Sprintf("stl_%d", time.Now().UnixNano())
}
