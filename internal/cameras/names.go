package cameras

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// NamesStore persists user-supplied display names for recordings.
type NamesStore struct {
	mu    sync.RWMutex
	file  string
	names map[string]string
}

// NewNamesStore creates or loads the display-name store.
func NewNamesStore() *NamesStore {
	_ = os.MkdirAll("recordings", 0o755)
	n := &NamesStore{
		file:  filepath.Join("recordings", "names.json"),
		names: make(map[string]string),
	}
	data, err := os.ReadFile(n.file)
	if err == nil {
		_ = json.Unmarshal(data, &n.names)
	}
	return n
}

// key returns the lookup key for a recording in a folder.
func (n *NamesStore) key(folder, filename string) string {
	folder = strings.Trim(folder, "/")
	return folder + "/" + filename
}

// Set stores a display name for a recording.
func (n *NamesStore) Set(folder, filename, name string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	k := n.key(folder, filename)
	if strings.TrimSpace(name) == "" {
		delete(n.names, k)
	} else {
		n.names[k] = strings.TrimSpace(name)
	}
	n.saveLocked()
}

// Get returns the stored display name, or the filename if none is set.
func (n *NamesStore) Get(folder, filename string) string {
	n.mu.RLock()
	defer n.mu.RUnlock()
	k := n.key(folder, filename)
	if v, ok := n.names[k]; ok && v != "" {
		return v
	}
	return filename
}

// GetAll returns a copy of all stored names.
func (n *NamesStore) GetAll() map[string]string {
	n.mu.RLock()
	defer n.mu.RUnlock()
	out := make(map[string]string, len(n.names))
	for k, v := range n.names {
		out[k] = v
	}
	return out
}

func (n *NamesStore) saveLocked() error {
	data, err := json.MarshalIndent(n.names, "", "  ")
	if err != nil {
		log.Printf("[record] marshal names error: %v", err)
		return err
	}
	data = append(data, '\n')
	if err := os.WriteFile(n.file, data, 0o600); err != nil {
		log.Printf("[record] write names error: %v", err)
		return err
	}
	return nil
}
