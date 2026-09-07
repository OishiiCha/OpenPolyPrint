// Package orcabridge stores data pushed by the OpenPolyPrint Bridge OrcaSlicer
// plugin: slicer instances (heartbeats) and the artifacts they sync (sliced
// G-code, preset bundles, settings snapshots). The UI scans these artifacts
// and can run a Gemini review over them.
package orcabridge

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// onlineWindow is how long an instance stays "online" after its last
// heartbeat. The plugin heartbeats every 30s by default.
const onlineWindow = 90 * time.Second

// maxGCodeArtifacts caps stored G-code artifacts; the plugin can upload one
// per slice, so the oldest entries are pruned automatically.
const maxGCodeArtifacts = 100

// Artifact types.
const (
	ArtifactGCode    = "gcode"
	ArtifactPresets  = "presets"
	ArtifactSettings = "settings"
)

// Instance is a slicer installation running the OpenPolyPrint Bridge plugin.
type Instance struct {
	InstanceID    string   `json:"instanceId"`
	Hostname      string   `json:"hostname"`
	Platform      string   `json:"platform,omitempty"`
	OrcaVersion   string   `json:"orcaVersion,omitempty"`
	PluginVersion string   `json:"pluginVersion,omitempty"`
	Capabilities  []string `json:"capabilities,omitempty"`
	FirstSeen     int64    `json:"firstSeen"`
	LastSeen      int64    `json:"lastSeen"`
	Online        bool     `json:"online"` // computed, not persisted
}

// Artifact is one synced item from a slicer instance.
type Artifact struct {
	ID           string            `json:"id"`
	InstanceID   string            `json:"instanceId"`
	InstanceHost string            `json:"instanceHost,omitempty"`
	Type         string            `json:"type"` // "gcode", "presets", "settings"
	Name         string            `json:"name"`
	Filename     string            `json:"filename,omitempty"`
	Size         int64             `json:"size"`
	GCodeLines   int               `json:"gcodeLines,omitempty"`
	Settings     map[string]string `json:"settings,omitempty"`
	Presets      json.RawMessage   `json:"presets,omitempty"` // loaded on Get, not in lists
	Analysis     string            `json:"analysis,omitempty"`
	AnalysisAt   int64             `json:"analyzedAt,omitempty"`
	CreatedAt    int64             `json:"createdAt"`
}

// Store manages bridge instances and artifacts in a directory.
type Store struct {
	mu        sync.RWMutex
	dir       string
	filesDir  string
	instances map[string]*Instance
	artifacts map[string]*Artifact
}

// NewStore creates a bridge store backed by the given directory.
func NewStore(dir string) (*Store, error) {
	filesDir := filepath.Join(dir, "files")
	if err := os.MkdirAll(filesDir, 0o755); err != nil {
		return nil, fmt.Errorf("create orcabridge dir: %w", err)
	}
	s := &Store{
		dir:       dir,
		filesDir:  filesDir,
		instances: make(map[string]*Instance),
		artifacts: make(map[string]*Artifact),
	}
	s.load()
	return s, nil
}

func (s *Store) load() {
	if data, err := os.ReadFile(filepath.Join(s.dir, "instances.json")); err == nil {
		var list []Instance
		if json.Unmarshal(data, &list) == nil {
			for i := range list {
				s.instances[list[i].InstanceID] = &list[i]
			}
		}
	}
	if data, err := os.ReadFile(filepath.Join(s.dir, "artifacts.json")); err == nil {
		var list []Artifact
		if json.Unmarshal(data, &list) == nil {
			for i := range list {
				s.artifacts[list[i].ID] = &list[i]
			}
		}
	}
}

func (s *Store) saveInstancesLocked() error {
	list := make([]Instance, 0, len(s.instances))
	for _, inst := range s.instances {
		view := *inst
		view.Online = false // transient, keep the persisted file clean
		list = append(list, view)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Hostname < list[j].Hostname })
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.dir, "instances.json"), append(data, '\n'), 0o600)
}

func (s *Store) saveArtifactsLocked() error {
	list := make([]Artifact, 0, len(s.artifacts))
	for _, a := range s.artifacts {
		list = append(list, *a)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].CreatedAt > list[j].CreatedAt })
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.dir, "artifacts.json"), append(data, '\n'), 0o600)
}

// Announce upserts a heartbeat from a slicer instance and returns the stored
// record (with FirstSeen preserved for repeat visitors).
func (s *Store) Announce(in Instance) Instance {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().Unix()
	if existing, ok := s.instances[in.InstanceID]; ok {
		in.FirstSeen = existing.FirstSeen
		if in.Hostname == "" {
			in.Hostname = existing.Hostname
		}
		if in.Platform == "" {
			in.Platform = existing.Platform
		}
		if in.OrcaVersion == "" {
			in.OrcaVersion = existing.OrcaVersion
		}
		if in.PluginVersion == "" {
			in.PluginVersion = existing.PluginVersion
		}
	} else {
		in.FirstSeen = now
	}
	in.LastSeen = now
	in.Online = true // it just heartbeated
	s.instances[in.InstanceID] = &in
	_ = s.saveInstancesLocked()
	return in
}

// ListInstances returns all instances with their computed online status.
func (s *Store) ListInstances() []Instance {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Instance, 0, len(s.instances))
	for _, inst := range s.instances {
		view := *inst
		view.Online = time.Since(time.Unix(view.LastSeen, 0)) < onlineWindow
		out = append(out, view)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Hostname < out[j].Hostname })
	return out
}

// AddArtifact stores a new artifact. content is the raw payload file (G-code
// bytes or presets JSON) and may be nil for metadata-only artifacts. G-code
// payloads are scanned for the embedded slicer config block.
func (s *Store) AddArtifact(a *Artifact, content []byte) (*Artifact, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().Unix()
	a.ID = fmt.Sprintf("ob_%d", now*1000+int64(time.Now().Nanosecond()/1e6))
	a.CreatedAt = now
	if len(content) > 0 {
		path := s.filePath(a.ID)
		if err := os.WriteFile(path, content, 0o644); err != nil {
			return nil, fmt.Errorf("write artifact file: %w", err)
		}
		a.Size = int64(len(content))
		if a.Type == ArtifactGCode {
			if settings, lines, err := ScanGCode(path); err == nil {
				a.Settings = settings
				a.GCodeLines = lines
			}
		}
	}
	s.artifacts[a.ID] = a
	if err := s.saveArtifactsLocked(); err != nil {
		if len(content) > 0 {
			_ = os.Remove(s.filePath(a.ID))
		}
		delete(s.artifacts, a.ID)
		return nil, err
	}
	s.pruneGCodeLocked()
	return a, nil
}

// PayloadPath returns the path of the artifact's payload file, or "" if the
// artifact doesn't exist or has no file.
func (s *Store) PayloadPath(id string) string {
	s.mu.RLock()
	_, ok := s.artifacts[id]
	s.mu.RUnlock()
	if !ok {
		return ""
	}
	path := s.filePath(id)
	if _, err := os.Stat(path); err != nil {
		return ""
	}
	return path
}

// pruneGCodeLocked drops the oldest G-code artifacts beyond maxGCodeArtifacts.
func (s *Store) pruneGCodeLocked() {
	gcode := make([]string, 0)
	for id, a := range s.artifacts {
		if a.Type == ArtifactGCode {
			gcode = append(gcode, id)
		}
	}
	if len(gcode) <= maxGCodeArtifacts {
		return
	}
	sort.Slice(gcode, func(i, j int) bool {
		return s.artifacts[gcode[i]].CreatedAt < s.artifacts[gcode[j]].CreatedAt
	})
	for _, id := range gcode[:len(gcode)-maxGCodeArtifacts] {
		_ = os.Remove(s.filePath(id))
		delete(s.artifacts, id)
	}
	_ = s.saveArtifactsLocked()
}

// ListArtifacts returns artifacts (newest first, optionally filtered by type)
// without their heavy payloads.
func (s *Store) ListArtifacts(artifactType string) []Artifact {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Artifact, 0, len(s.artifacts))
	for _, a := range s.artifacts {
		if artifactType != "" && a.Type != artifactType {
			continue
		}
		view := *a
		view.Presets = nil
		out = append(out, view)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	return out
}

// GetArtifact returns a single artifact with its presets payload loaded.
func (s *Store) GetArtifact(id string) (*Artifact, error) {
	s.mu.RLock()
	a, ok := s.artifacts[id]
	s.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	view := *a
	if a.Type == ArtifactPresets {
		if data, err := os.ReadFile(s.filePath(id)); err == nil {
			view.Presets = json.RawMessage(data)
		}
	}
	return &view, nil
}

// Remove deletes an artifact and its payload file.
func (s *Store) Remove(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.artifacts[id]; !ok {
		return false
	}
	_ = os.Remove(s.filePath(id))
	delete(s.artifacts, id)
	_ = s.saveArtifactsLocked()
	return true
}

// Content returns the raw payload file for download.
func (s *Store) Content(id string) ([]byte, string, error) {
	s.mu.RLock()
	a, ok := s.artifacts[id]
	s.mu.RUnlock()
	if !ok {
		return nil, "", fmt.Errorf("not found")
	}
	data, err := os.ReadFile(s.filePath(id))
	if err != nil {
		return nil, "", err
	}
	name := a.Filename
	if name == "" {
		name = a.ID
		if a.Type == ArtifactGCode {
			name += ".gcode"
		} else if a.Type == ArtifactPresets {
			name += ".json"
		}
	}
	return data, name, nil
}

// SaveAnalysis stores the AI review text on an artifact.
func (s *Store) SaveAnalysis(id, analysis string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.artifacts[id]
	if !ok {
		return false
	}
	a.Analysis = analysis
	a.AnalysisAt = time.Now().Unix()
	_ = s.saveArtifactsLocked()
	return true
}

func (s *Store) filePath(id string) string {
	return filepath.Join(s.filesDir, id)
}

// ScanGCode reads a G-code file and extracts the embedded slicer config plus
// the total line count. OrcaSlicer/Bambu Studio write a `; key = value`
// config block (and PrusaSlicer-style files carry the full config as trailing
// comments); all such comment lines are collected, later occurrences winning,
// so the trailing block overrides anything earlier in the file.
func ScanGCode(path string) (map[string]string, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	settings := make(map[string]string)
	lines := 0
	for scanner.Scan() {
		lines++
		line := scanner.Text()
		if !strings.HasPrefix(line, ";") {
			continue
		}
		rest := strings.TrimSpace(line[1:])
		idx := strings.Index(rest, "=")
		if idx <= 0 {
			continue
		}
		key := strings.TrimSpace(rest[:idx])
		if !isSettingsKey(key) {
			continue
		}
		settings[key] = strings.TrimSpace(rest[idx+1:])
	}
	if err := scanner.Err(); err != nil {
		return settings, lines, err
	}
	return settings, lines, nil
}

// isSettingsKey filters comment lines to identifier-like keys so prose
// comments containing "=" don't pollute the settings map.
func isSettingsKey(key string) bool {
	if len(key) < 2 || len(key) > 64 {
		return false
	}
	for i, r := range key {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
		case r == '_', r == '.', r == '-':
		case i > 0 && r >= '0' && r <= '9':
		default:
			return false
		}
	}
	return true
}

// Excerpt returns the first head and last tail lines of a file for AI
// context, with a note about skipped lines in between. G-code can be tens of
// megabytes, so it is scanned streaming rather than loaded whole.
func Excerpt(path string, head, tail int) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var headLines []string
	tailWindow := make([]string, 0, tail)
	total := 0
	for scanner.Scan() {
		total++
		line := scanner.Text()
		if len(headLines) < head {
			headLines = append(headLines, line)
		}
		if tail > 0 {
			if len(tailWindow) == tail {
				tailWindow = tailWindow[1:]
			}
			tailWindow = append(tailWindow, line)
		}
	}
	if total <= head+tail {
		return strings.Join(headLines, "\n")
	}
	skipped := total - len(headLines) - len(tailWindow)
	var sb strings.Builder
	sb.WriteString(strings.Join(headLines, "\n"))
	sb.WriteString(fmt.Sprintf("\n\n; ... %d lines omitted ... \n\n", skipped))
	sb.WriteString(strings.Join(tailWindow, "\n"))
	return sb.String()
}
