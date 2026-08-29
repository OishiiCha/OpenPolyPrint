package ai

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// ChatMessage is a single message in an AI chat conversation.
type ChatMessage struct {
	Role       string    `json:"role"`                 // "user" or "model"
	Text       string    `json:"text"`                 // message text
	HasImage   bool      `json:"hasImage"`             // true if this message includes captured frame(s)
	ImagePaths []string  `json:"imagePaths,omitempty"` // relative paths to saved images
	ImageMime  string    `json:"imageMime,omitempty"`
	Timestamp  time.Time `json:"timestamp"`
}

// ChatConversation is a full conversation tied to a printer/print session.
type ChatConversation struct {
	ID          string        `json:"id"`
	PrinterID   string        `json:"printerId"`
	PrinterName string        `json:"printerName"`
	File        string        `json:"file,omitempty"`
	CreatedAt   time.Time     `json:"createdAt"`
	Messages    []ChatMessage `json:"messages"`
}

// ChatStore persists AI chat conversations to disk.
type ChatStore struct {
	mu    sync.RWMutex
	dir   string
	convs map[string]*ChatConversation
}

// NewChatStore creates or loads a chat store from the given directory.
func NewChatStore(dir string) *ChatStore {
	s := &ChatStore{
		dir:   dir,
		convs: make(map[string]*ChatConversation),
	}
	_ = os.MkdirAll(dir, 0o755)
	s.loadAll()
	return s
}

func (s *ChatStore) loadAll() {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.dir, e.Name()))
		if err != nil {
			continue
		}
		var conv ChatConversation
		if json.Unmarshal(data, &conv) == nil {
			s.convs[conv.ID] = &conv
		}
	}
}

func (s *ChatStore) save(conv *ChatConversation) error {
	data, err := json.MarshalIndent(conv, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.dir, conv.ID+".json"), data, 0o600)
}

// Create starts a new conversation for the given printer.
func (s *ChatStore) Create(printerID, printerName, file string) *ChatConversation {
	id := time.Now().Format("20060102150405.000")
	conv := &ChatConversation{
		ID:          id,
		PrinterID:   printerID,
		PrinterName: printerName,
		File:        file,
		CreatedAt:   time.Now(),
	}
	s.mu.Lock()
	s.convs[id] = conv
	s.mu.Unlock()
	_ = s.save(conv)
	return conv
}

// Get returns a conversation by ID.
func (s *ChatStore) Get(id string) *ChatConversation {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.convs[id]
}

// List returns all conversations, newest first.
func (s *ChatStore) List() []*ChatConversation {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*ChatConversation, 0, len(s.convs))
	for _, c := range s.convs {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

// AddMessage appends a message to a conversation and saves.
func (s *ChatStore) AddMessage(convID string, msg ChatMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	conv, ok := s.convs[convID]
	if !ok {
		return os.ErrNotExist
	}
	conv.Messages = append(conv.Messages, msg)
	return s.save(conv)
}

// Delete removes a conversation and its images.
func (s *ChatStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	conv, ok := s.convs[id]
	if !ok {
		return os.ErrNotExist
	}
	// Remove image files
	for _, msg := range conv.Messages {
		if msg.HasImage {
			for _, p := range msg.ImagePaths {
				_ = os.Remove(filepath.Join(s.dir, p))
			}
		}
	}
	_ = os.Remove(filepath.Join(s.dir, id+".json"))
	delete(s.convs, id)
	return nil
}

// SaveImage writes image bytes to the chat directory and returns the relative path.
func (s *ChatStore) SaveImage(convID string, data []byte, ext string) (string, error) {
	filename := convID + "_" + time.Now().Format("150405.000") + ext
	path := filepath.Join(s.dir, filename)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", err
	}
	return filename, nil
}

// ImagePath returns the full path for a relative image path.
func (s *ChatStore) ImagePath(rel string) string {
	return filepath.Join(s.dir, rel)
}
