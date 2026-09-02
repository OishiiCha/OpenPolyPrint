package anker

import (
	"encoding/json"
	"log"
	"sync"
	"time"
)

// PrinterState holds the latest known printer state from MQTT messages.
// It is a self-contained copy of the upstream web printer state logic.
type PrinterState struct {
	mu        sync.RWMutex
	connected bool

	// Print status (commandType 1001)
	FileName    string
	Progress    float64
	TimeElapsed int64
	TimeRemain  int64
	TotalTime   int64

	// Temperatures (commandType 1003)
	NozzleTemp    float64
	SetNozzleTemp float64
	BedTemp       float64
	SetBedTemp    float64

	// Print speed / layers (commandType 1005/1006)
	PrintSpeed float64 // mm/s
	LayerNum   int
	LayerCount int

	// Used filament in mm (commandType 1001 or 1006 — field name TBD)
	UsedFilament float64

	// State tracking
	State        string // idle, printing, paused
	StateUpdated time.Time
	LastUpdate   time.Time
}

// NewPrinterState creates a new PrinterState.
func NewPrinterState() *PrinterState {
	return &PrinterState{
		State: "idle",
	}
}

// UpdateFromMQTT processes an MQTT message map and updates state.
func (ps *PrinterState) UpdateFromMQTT(data map[string]any) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.LastUpdate = time.Now()

	ct, _ := data["commandType"].(float64)

	// Log all fields for commandTypes we care about, so we can discover
	// what the printer actually sends (used filament, speed, etc.)
	if ct == 1001 || ct == 1005 || ct == 1006 || ct == 1007 || ct == 1068 || ct == 1085 {
		if js, err := json.Marshal(data); err == nil {
			log.Printf("[anker-state] ct=%d fields=%s", int(ct), string(js))
		}
	}

	switch int(ct) {
	case 1001:
		if name, ok := data["name"].(string); ok {
			ps.FileName = name
		}
		if progress, ok := data["progress"].(float64); ok {
			ps.Progress = progress
		}
		if totalTime, ok := data["totalTime"].(float64); ok {
			ps.TotalTime = int64(totalTime)
		}
		if remain, ok := data["time"].(float64); ok {
			ps.TimeRemain = int64(remain)
			ps.TimeElapsed = ps.TotalTime - int64(remain)
		}
		// Used filament — try common field names
		if uf, ok := data["usedFilament"].(float64); ok {
			ps.UsedFilament = uf
		} else if uf, ok := data["filamentUsed"].(float64); ok {
			ps.UsedFilament = uf
		} else if uf, ok := data["used"].(float64); ok {
			ps.UsedFilament = uf
		}
		if ps.Progress > 0 && ps.Progress < 10000 {
			ps.State = "printing"
		} else if ps.Progress >= 10000 {
			ps.State = "idle"
		}
		ps.StateUpdated = time.Now()

	case 1003:
		if current, ok := data["currentTemp"].(float64); ok {
			ps.NozzleTemp = current / 100
		}
		if target, ok := data["targetTemp"].(float64); ok {
			ps.SetNozzleTemp = target / 100
		}

	case 1004:
		if current, ok := data["currentTemp"].(float64); ok {
			ps.BedTemp = current / 100
		}
		if target, ok := data["targetTemp"].(float64); ok {
			ps.SetBedTemp = target / 100
		}

	case 1005:
		// 1005 is fan speed, not print speed — but some firmware versions
		// may send print speed here too
		if speed, ok := data["fanSpeed"].(float64); ok {
			// Don't overwrite print speed with fan speed
			_ = speed
		}
		if speed, ok := data["printSpeed"].(float64); ok {
			ps.PrintSpeed = speed
		}
		if speed, ok := data["value"].(float64); ok {
			ps.PrintSpeed = speed
		}

	case 1006:
		// Print speed (mm/s) and layer info
		if speed, ok := data["value"].(float64); ok {
			ps.PrintSpeed = speed
		}
		if speed, ok := data["printSpeed"].(float64); ok {
			ps.PrintSpeed = speed
		}
		if layer, ok := data["layerNum"].(float64); ok {
			ps.LayerNum = int(layer)
		}
		if count, ok := data["layerCount"].(float64); ok {
			ps.LayerCount = int(count)
		}
		if uf, ok := data["usedFilament"].(float64); ok {
			ps.UsedFilament = uf
		}

	case 1068, 1085, 1192:
		// Additional status message types seen in the protocol —
		// log them and try to extract useful fields
		if speed, ok := data["printSpeed"].(float64); ok {
			ps.PrintSpeed = speed
		}
		if uf, ok := data["usedFilament"].(float64); ok {
			ps.UsedFilament = uf
		} else if uf, ok := data["filamentUsed"].(float64); ok {
			ps.UsedFilament = uf
		}
		if layer, ok := data["layerNum"].(float64); ok {
			ps.LayerNum = int(layer)
		}
		if count, ok := data["layerCount"].(float64); ok {
			ps.LayerCount = int(count)
		}
	}
}

// SetConnected updates the connection status.
func (ps *PrinterState) SetConnected(connected bool) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.connected = connected
}

// SetState updates the printer state.
func (ps *PrinterState) SetState(state string) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.State = state
	ps.StateUpdated = time.Now()
}

// Snapshot returns a read-only copy of the current state.
func (ps *PrinterState) Snapshot() map[string]any {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	return map[string]any{
		"connected":     ps.connected,
		"state":         ps.State,
		"fileName":      ps.FileName,
		"progress":      ps.Progress,
		"timeElapsed":   ps.TimeElapsed,
		"timeRemaining": ps.TimeRemain,
		"totalTime":     ps.TotalTime,
		"nozzleTemp":    ps.NozzleTemp,
		"setNozzleTemp": ps.SetNozzleTemp,
		"bedTemp":       ps.BedTemp,
		"setBedTemp":    ps.SetBedTemp,
		"printSpeed":    ps.PrintSpeed,
		"usedFilament":  ps.UsedFilament,
		"layerNum":      ps.LayerNum,
		"layerCount":    ps.LayerCount,
		"lastUpdate":    ps.LastUpdate.Format(time.RFC3339),
	}
}
