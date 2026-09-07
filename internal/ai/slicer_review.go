package ai

import (
	"fmt"
	"sort"
	"strings"
)

// SlicerReviewRequest carries everything the AI needs to review a slicer
// artifact synced through the OrcaSlicer bridge: a G-code excerpt, the
// resolved slicer settings, and/or a preset bundle snapshot.
type SlicerReviewRequest struct {
	APIKey       string
	ArtifactType string // "gcode", "presets", "settings"
	Name         string
	InstanceHost string
	GCodeExcerpt string
	GCodeLines   int
	Settings     map[string]string
	PresetsJSON  string
	CustomPrompt string
}

// ReviewSlicerArtifact asks Gemini to review a synced slicer artifact and
// returns a markdown report (summary, potential issues, recommendations).
func ReviewSlicerArtifact(req SlicerReviewRequest) (*ChatResponse, error) {
	if req.APIKey == "" {
		return nil, fmt.Errorf("API key required")
	}
	prompt := buildSlicerReviewPrompt(req)
	return Chat(ChatRequest{
		APIKey: req.APIKey,
		Messages: []ChatMessageForAPI{
			{Role: "user", Parts: []ChatPart{{Text: prompt}}},
		},
	})
}

const (
	maxSettingsChars = 6000
	maxPresetsChars  = 8000
)

// buildSlicerReviewPrompt assembles the review prompt, including only the
// sections that have data.
func buildSlicerReviewPrompt(req SlicerReviewRequest) string {
	var sb strings.Builder
	sb.WriteString("You are a meticulous 3D printing slicer expert. Review the following slicer artifact")
	switch req.ArtifactType {
	case "gcode":
		sb.WriteString(" (sliced G-code with its resolved settings)")
	case "presets":
		sb.WriteString(" (a slicer preset bundle snapshot)")
	case "settings":
		sb.WriteString(" (a slicer settings snapshot)")
	}
	sb.WriteString(" and report configuration problems, print-quality risks, and safety concerns before the user prints it.\n\n")

	sb.WriteString("## Artifact\n")
	if req.Name != "" {
		sb.WriteString(fmt.Sprintf("- Name: %s\n", req.Name))
	}
	if req.InstanceHost != "" {
		sb.WriteString(fmt.Sprintf("- Slicer host: %s\n", req.InstanceHost))
	}
	if req.GCodeLines > 0 {
		sb.WriteString(fmt.Sprintf("- G-code lines: %d\n", req.GCodeLines))
	}
	sb.WriteString("\n")

	if len(req.Settings) > 0 {
		sb.WriteString("## Resolved Slicer Settings\n")
		sb.WriteString("```\n")
		sb.WriteString(formatSettings(req.Settings, maxSettingsChars))
		sb.WriteString("\n```\n\n")
	}

	if strings.TrimSpace(req.PresetsJSON) != "" {
		sb.WriteString("## Preset Bundle Snapshot\n")
		sb.WriteString("```json\n")
		sb.WriteString(truncate(strings.TrimSpace(req.PresetsJSON), maxPresetsChars))
		sb.WriteString("\n```\n\n")
	}

	if req.GCodeExcerpt != "" {
		sb.WriteString("## G-code (start and end excerpt)\n")
		sb.WriteString("```\n")
		sb.WriteString(truncate(req.GCodeExcerpt, 10000))
		sb.WriteString("\n```\n\n")
	}

	sb.WriteString("## What to Check\n")
	sb.WriteString("1. Material/temperature sanity: nozzle and bed temperatures vs filament type, chamber/fan settings for the material\n")
	sb.WriteString("2. Geometry/mechanics: layer height vs nozzle diameter, line widths, speeds and acceleration (esp. first layer), volumetric flow limits\n")
	sb.WriteString("3. Adhesion and cooling: bed adhesion (brim/raft needs), part cooling fan ramp, minimum layer time for overhangs/small features\n")
	sb.WriteString("4. Retraction and stringing: retraction length/speed, wipe/tower settings where relevant\n")
	sb.WriteString("5. Supports and overhangs: missing supports for obvious overhangs, top/bottom shell thickness vs infill exposure\n")
	sb.WriteString("6. G-code safety (when provided): heaters commanded before moves, homing present, sane purge/prime, temperatures never absurd (e.g. >300C for PLA, bed >120C), end gcode parks and turns off heaters\n")
	sb.WriteString("7. Internal consistency: settings that contradict each other (e.g. high speed + low volumetric limit, sparse infill + single top surface)\n\n")

	sb.WriteString("## Response Format (markdown)\n")
	sb.WriteString("## Summary\n2-4 sentences on what this artifact is and its overall readiness.\n\n")
	sb.WriteString("## Potential Issues\nA bullet list. Each bullet: severity tag [high]/[medium]/[low], a short title, then the specific setting(s) involved with current values, and the concrete risk. Write 'None detected' if the configuration is sound.\n\n")
	sb.WriteString("## Recommendations\nA bullet list of specific, actionable changes (exact setting keys and suggested values). Include print-quality and reliability improvements, ranked by impact. If nothing needs changing, say so.\n")

	if req.CustomPrompt != "" {
		sb.WriteString("\n## Additional User Instructions\n")
		sb.WriteString(req.CustomPrompt)
		sb.WriteString("\n")
	}
	return sb.String()
}

// formatSettings renders the settings map as sorted `key = value` lines,
// truncated to limit characters.
func formatSettings(settings map[string]string, limit int) string {
	keys := make([]string, 0, len(settings))
	for k := range settings {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	for _, k := range keys {
		line := k + " = " + settings[k]
		if sb.Len()+len(line) > limit {
			sb.WriteString(fmt.Sprintf("; ... %d more settings omitted ...\n", len(keys)))
			break
		}
		sb.WriteString(line)
		sb.WriteString("\n")
	}
	return strings.TrimSuffix(sb.String(), "\n")
}

func truncate(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	return s[:limit] + fmt.Sprintf("\n; ... truncated, %d more chars omitted ...", len(s)-limit)
}
