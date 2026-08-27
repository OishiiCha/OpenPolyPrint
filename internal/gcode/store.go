package gcode

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// File is the JSON representation of a stored G-code file.
type File struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Size      string `json:"size"`
	Path      string `json:"-"`
	FileSize  int64  `json:"fileSize"`
	PrinterID string `json:"printerId,omitempty"`
}

type fileMeta struct {
	PrinterID string `json:"printerId,omitempty"`
}

// Store keeps G-code files in a directory.
type Store struct {
	dir string
}

// NewStore creates a G-code store backed by the given directory.
func NewStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create gcode dir: %w", err)
	}
	return &Store{dir: dir}, nil
}

func humanSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	units := []string{"KB", "MB", "GB"}
	return fmt.Sprintf("%.1f %s", float64(bytes)/float64(div), units[exp])
}

func (s *Store) metaPath() string {
	return filepath.Join(s.dir, "gcode.json")
}

func (s *Store) loadMeta() (map[string]fileMeta, error) {
	data, err := os.ReadFile(s.metaPath())
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]fileMeta{}, nil
		}
		return nil, fmt.Errorf("read gcode metadata: %w", err)
	}
	var m map[string]fileMeta
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse gcode metadata: %w", err)
	}
	return m, nil
}

func (s *Store) saveMeta(m map[string]fileMeta) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal gcode metadata: %w", err)
	}
	return os.WriteFile(s.metaPath(), append(data, '\n'), 0o600)
}

func (s *Store) fileToEntry(path string, meta map[string]fileMeta) (File, error) {
	info, err := os.Stat(path)
	if err != nil {
		return File{}, err
	}
	name := info.Name()
	return File{
		ID:        name,
		Name:      name,
		Size:      humanSize(info.Size()),
		Path:      path,
		FileSize:  info.Size(),
		PrinterID: meta[name].PrinterID,
	}, nil
}

// List returns the stored G-code files sorted by name.
func (s *Store) List() ([]File, error) {
	meta, err := s.loadMeta()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, fmt.Errorf("read gcode dir: %w", err)
	}
	var files []File
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if e.Name() == "gcode.json" {
			continue
		}
		path := filepath.Join(s.dir, e.Name())
		f, err := s.fileToEntry(path, meta)
		if err != nil {
			continue
		}
		files = append(files, f)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })
	return files, nil
}

// Save writes an uploaded file to the store and records the associated printer.
func (s *Store) Save(name, printerID string, r io.Reader) (File, error) {
	if name == "" {
		return File{}, fmt.Errorf("filename required")
	}
	base := filepath.Base(name)
	path := filepath.Join(s.dir, base)
	out, err := os.Create(path)
	if err != nil {
		return File{}, fmt.Errorf("create file: %w", err)
	}
	_, err = io.Copy(out, r)
	_ = out.Close()
	if err != nil {
		_ = os.Remove(path)
		return File{}, fmt.Errorf("write file: %w", err)
	}

	meta, err := s.loadMeta()
	if err != nil {
		return File{}, err
	}
	meta[base] = fileMeta{PrinterID: printerID}
	if err := s.saveMeta(meta); err != nil {
		return File{}, err
	}
	f, err := s.fileToEntry(path, meta)
	return f, err
}

// Load reads a G-code file by id (filename).
func (s *Store) Load(id string) ([]byte, error) {
	path := s.FilePath(id)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}
	return data, nil
}

// FilePath returns the on-disk path for a G-code file by ID.
func (s *Store) FilePath(id string) string {
	name, err := url.PathUnescape(id)
	if err != nil {
		name = id
	}
	base := filepath.Base(name)
	return filepath.Join(s.dir, base)
}

// Timeline parses a G-code file and returns timestamped segments for
// visualization and AI analysis.
func (s *Store) Timeline(id string) ([]Segment, error) {
	data, err := s.Load(id)
	if err != nil {
		return nil, err
	}
	return ParseTimeline(strings.NewReader(string(data)))
}

// Delete removes a G-code file by id (filename).
func (s *Store) Delete(id string) error {
	name, err := url.PathUnescape(id)
	if err != nil {
		name = id
	}
	base := filepath.Base(name)
	path := filepath.Join(s.dir, base)
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("remove file: %w", err)
	}
	meta, err := s.loadMeta()
	if err != nil {
		return err
	}
	delete(meta, base)
	return s.saveMeta(meta)
}
