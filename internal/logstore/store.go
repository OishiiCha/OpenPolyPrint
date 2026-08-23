package logstore

import (
	"bytes"
	"io"
	"strings"
	"sync"
)

// Store is a concurrent ring buffer of log lines.
type Store struct {
	mu      sync.Mutex
	lines   []string
	cap     int
	tail    []byte
	limited io.Writer
}

// New creates a log store that keeps at most capacity complete lines.
func New(capacity int) *Store {
	return &Store{
		lines: make([]string, 0, capacity),
		cap:   capacity,
	}
}

// Write implements io.Writer. It buffers partial lines until a newline arrives.
func (s *Store) Write(p []byte) (n int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data := append(s.tail, p...)
	for {
		idx := bytes.IndexByte(data, '\n')
		if idx < 0 {
			s.tail = data
			break
		}
		line := strings.TrimSuffix(string(data[:idx]), "\r")
		s.lines = append(s.lines, line)
		if len(s.lines) > s.cap {
			s.lines = s.lines[len(s.lines)-s.cap:]
		}
		data = data[idx+1:]
	}
	return len(p), nil
}

// Lines returns the currently stored lines (oldest first).
func (s *Store) Lines() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.lines))
	copy(out, s.lines)
	return out
}

// String returns all stored lines as a single newline-delimited string.
func (s *Store) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return strings.Join(s.lines, "\n")
}

// Clear removes all stored lines.
func (s *Store) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lines = s.lines[:0]
	s.tail = nil
}
