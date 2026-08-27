package printsession

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Session tracks data about an active print session for AI analysis.
// It logs temperature samples, printer status changes, and metadata
// to a JSON file that can be used with the AI analysis feature.
type Session struct {
	PrinterID   string     `json:"printerId"`
	PrinterName string     `json:"printerName"`
	FileName    string     `json:"fileName"`
	StartTime   time.Time  `json:"startTime"`
	EndTime     *time.Time `json:"endTime,omitempty"`
	Status      string     `json:"status"`
	Progress    float64    `json:"progress"`

	TempSamples []TempSample  `json:"tempSamples"`
	StatusLog   []StatusEntry `json:"statusLog"`

	mu       sync.Mutex
	filePath string
	stopCh   chan struct{}
	done     chan struct{}
}

// TempSample is a single temperature reading during a print.
type TempSample struct {
	Time         int64   `json:"time"`
	Nozzle       float64 `json:"nozzle"`
	TargetNozzle float64 `json:"targetNozzle"`
	Bed          float64 `json:"bed"`
	TargetBed    float64 `json:"targetBed"`
	Progress     float64 `json:"progress"`
}

// StatusEntry records a status change during the print.
type StatusEntry struct {
	Time     int64   `json:"time"`
	Status   string  `json:"status"`
	Progress float64 `json:"progress"`
	File     string  `json:"file"`
}

// Manager tracks active print sessions per printer.
type Manager struct {
	mu       sync.Mutex
	sessions map[string]*Session
	dataDir  string
}

// NewManager creates a new print session manager.
// dataDir is where session JSON files are saved (e.g. "recordings/sessions").
func NewManager(dataDir string) *Manager {
	if dataDir == "" {
		dataDir = "recordings/sessions"
	}
	return &Manager{
		sessions: make(map[string]*Session),
		dataDir:  dataDir,
	}
}

// Start begins tracking a print session for the given printer.
// If a session is already active for this printer, it returns it.
func (m *Manager) Start(printerID, printerName, fileName string) *Session {
	m.mu.Lock()
	defer m.mu.Unlock()

	if existing, ok := m.sessions[printerID]; ok {
		return existing
	}

	if err := os.MkdirAll(m.dataDir, 0o755); err != nil {
		log.Printf("[printsession] create dir: %v", err)
		return nil
	}

	safeName := safeFilename(printerName)
	safeFile := safeFilename(fileName)
	if safeFile == "" {
		safeFile = "unknown"
	}
	timestamp := time.Now().Format("20060102-150405")
	filePath := filepath.Join(m.dataDir, fmt.Sprintf("%s_%s_%s.json", safeName, safeFile, timestamp))

	sess := &Session{
		PrinterID:   printerID,
		PrinterName: printerName,
		FileName:    fileName,
		StartTime:   time.Now(),
		Status:      "Printing",
		TempSamples: []TempSample{},
		StatusLog: []StatusEntry{
			{Time: time.Now().Unix(), Status: "Printing", Progress: 0, File: fileName},
		},
		filePath: filePath,
		stopCh:   make(chan struct{}),
		done:     make(chan struct{}),
	}

	m.sessions[printerID] = sess
	sess.save()
	log.Printf("[printsession] started session for %s: %s", printerName, filePath)
	return sess
}

// RecordTemp adds a temperature sample to the active session.
func (m *Manager) RecordTemp(printerID string, nozzle, targetNozzle, bed, targetBed, progress float64) {
	m.mu.Lock()
	sess, ok := m.sessions[printerID]
	m.mu.Unlock()
	if !ok {
		return
	}

	sess.mu.Lock()
	sess.TempSamples = append(sess.TempSamples, TempSample{
		Time:         time.Now().Unix(),
		Nozzle:       nozzle,
		TargetNozzle: targetNozzle,
		Bed:          bed,
		TargetBed:    targetBed,
		Progress:     progress,
	})
	sess.Progress = progress
	sess.mu.Unlock()
}

// RecordStatus updates the status of an active session and logs changes.
func (m *Manager) RecordStatus(printerID, status string, progress float64, file string) {
	m.mu.Lock()
	sess, ok := m.sessions[printerID]
	m.mu.Unlock()
	if !ok {
		return
	}

	sess.mu.Lock()
	if sess.Status != status {
		sess.StatusLog = append(sess.StatusLog, StatusEntry{
			Time:     time.Now().Unix(),
			Status:   status,
			Progress: progress,
			File:     file,
		})
		sess.Status = status
	}
	sess.Progress = progress
	sess.mu.Unlock()
}

// Stop ends the session for the given printer and saves the final file.
func (m *Manager) Stop(printerID, finalStatus string) *Session {
	m.mu.Lock()
	sess, ok := m.sessions[printerID]
	if !ok {
		m.mu.Unlock()
		return nil
	}
	delete(m.sessions, printerID)
	m.mu.Unlock()

	sess.mu.Lock()
	now := time.Now()
	sess.EndTime = &now
	sess.Status = finalStatus
	sess.StatusLog = append(sess.StatusLog, StatusEntry{
		Time:   now.Unix(),
		Status: finalStatus,
		File:   sess.FileName,
	})
	sess.mu.Unlock()

	sess.save()
	log.Printf("[printsession] stopped session for %s: %s", sess.PrinterName, sess.filePath)
	return sess
}

// Get returns the active session for a printer, if any.
func (m *Manager) Get(printerID string) *Session {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sessions[printerID]
}

// IsActive returns true if there's an active session for the printer.
func (m *Manager) IsActive(printerID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.sessions[printerID]
	return ok
}

// ActiveSessions returns info about all active sessions.
func (m *Manager) ActiveSessions() []SessionInfo {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]SessionInfo, 0, len(m.sessions))
	for _, s := range m.sessions {
		s.mu.Lock()
		out = append(out, SessionInfo{
			PrinterID:   s.PrinterID,
			PrinterName: s.PrinterName,
			FileName:    s.FileName,
			StartTime:   s.StartTime,
			Status:      s.Status,
			Progress:    s.Progress,
			SampleCount: len(s.TempSamples),
		})
		s.mu.Unlock()
	}
	return out
}

// SessionInfo is a summary of an active session (without full temp data).
type SessionInfo struct {
	PrinterID   string    `json:"printerId"`
	PrinterName string    `json:"printerName"`
	FileName    string    `json:"fileName"`
	StartTime   time.Time `json:"startTime"`
	Status      string    `json:"status"`
	Progress    float64   `json:"progress"`
	SampleCount int       `json:"sampleCount"`
}

// GetSessionFile returns the path to the session JSON file for a printer.
func (m *Manager) GetSessionFile(printerID string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if sess, ok := m.sessions[printerID]; ok {
		return sess.filePath
	}
	return ""
}

// save writes the session to its JSON file.
func (s *Session) save() {
	s.mu.Lock()
	data, err := json.MarshalIndent(s, "", "  ")
	s.mu.Unlock()
	if err != nil {
		log.Printf("[printsession] marshal: %v", err)
		return
	}
	if err := os.WriteFile(s.filePath, data, 0o644); err != nil {
		log.Printf("[printsession] write: %v", err)
	}
}

func safeFilename(s string) string {
	var b []byte
	for _, c := range s {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '-' {
			b = append(b, byte(c))
		} else if c == ' ' {
			b = append(b, '_')
		}
	}
	return string(b)
}

// SavedSessionInfo is metadata about a saved session file on disk.
type SavedSessionInfo struct {
	ID           string     `json:"id"` // filename without .json
	PrinterName  string     `json:"printerName"`
	FileName     string     `json:"fileName"`
	StartTime    time.Time  `json:"startTime"`
	EndTime      *time.Time `json:"endTime,omitempty"`
	Status       string     `json:"status"`
	Progress     float64    `json:"progress"`
	SampleCount  int        `json:"sampleCount"`
	TimelapseDir string     `json:"timelapseDir,omitempty"` // linked timelapse frames dir
	HasGcode     bool       `json:"hasGcode"`               // whether a matching gcode file exists
	GcodeID      string     `json:"gcodeId,omitempty"`      // matching gcode file ID
}

// ListSavedSessions lists all saved session files with metadata.
// It also attempts to link each session to a timelapse frame directory
// by matching the timestamp in the filename.
func (m *Manager) ListSavedSessions() ([]SavedSessionInfo, error) {
	entries, err := os.ReadDir(m.dataDir)
	if err != nil {
		return nil, err
	}

	// Build a set of available timelapse frame dirs for linking
	timelapseDirs := map[string]bool{}
	if tlEntries, err := os.ReadDir("recordings/timelapse"); err == nil {
		for _, e := range tlEntries {
			if e.IsDir() && strings.HasSuffix(e.Name(), "_frames") {
				timelapseDirs[e.Name()] = true
			}
		}
	}

	var sessions []SavedSessionInfo
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".json")
		data, err := os.ReadFile(filepath.Join(m.dataDir, e.Name()))
		if err != nil {
			continue
		}
		var sess Session
		if err := json.Unmarshal(data, &sess); err != nil {
			continue
		}
		info := SavedSessionInfo{
			ID:          id,
			PrinterName: sess.PrinterName,
			FileName:    sess.FileName,
			StartTime:   sess.StartTime,
			EndTime:     sess.EndTime,
			Status:      sess.Status,
			Progress:    sess.Progress,
			SampleCount: len(sess.TempSamples),
		}

		// Try to find linked timelapse dir by timestamp
		timestamp := sess.StartTime.Format("20060102-150405")
		for dir := range timelapseDirs {
			if strings.Contains(dir, timestamp) {
				info.TimelapseDir = dir
				break
			}
		}

		sessions = append(sessions, info)
	}

	// Sort by start time descending (newest first)
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].StartTime.After(sessions[j].StartTime)
	})

	return sessions, nil
}

// GetSavedSession loads a session by its ID (filename without .json).
func (m *Manager) GetSavedSession(id string) (*Session, error) {
	filePath := filepath.Join(m.dataDir, id+".json")
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("load session: %w", err)
	}
	var sess Session
	if err := json.Unmarshal(data, &sess); err != nil {
		return nil, fmt.Errorf("parse session: %w", err)
	}
	return &sess, nil
}
