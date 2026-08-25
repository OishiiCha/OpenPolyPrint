package printers

import (
	"context"
	"time"
)

// Temps is the unified temperature snapshot for any supported printer.
type Temps struct {
	Nozzle       float64 `json:"nozzle"`
	Bed          float64 `json:"bed"`
	TargetNozzle float64 `json:"targetNozzle"`
	TargetBed    float64 `json:"targetBed"`
}

// Status is the unified status snapshot for any supported printer.
type Status struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Type          string    `json:"type"`
	Online        bool      `json:"online"`
	State         string    `json:"state"`  // raw internal state
	StatusText    string    `json:"status"` // user-facing: Idle / Printing / Paused / Offline
	Temps         Temps     `json:"temps"`
	Progress      int       `json:"progress"`
	CurrentFile   string    `json:"currentFile,omitempty"`
	RemainingTime string    `json:"remainingTime,omitempty"`
	UpdatedAt     time.Time `json:"updated_at"`
	Error         string    `json:"error,omitempty"`
}

// PrinterConfig is the user-supplied configuration for a printer.
type PrinterConfig struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Type   string `json:"type"`
	Host   string `json:"host,omitempty"`
	APIKey string `json:"apiKey,omitempty"`
}

// Driver is the common interface every printer provider must implement.
type Driver interface {
	PrinterID() string
	Name() string
	Type() string
	Connect(ctx context.Context) error
	Disconnect() error
	Status() (Status, error)
	PausePrint(ctx context.Context) error
	StopPrint(ctx context.Context) error
	Home(ctx context.Context) error
	Preheat(ctx context.Context, nozzle, bed float64) error
	Cooldown(ctx context.Context) error
	AutoLevel(ctx context.Context) error
	SendGCode(ctx context.Context, command string) error
	// UploadGCode sends a G-code file to the printer. The filename is the
	// user-facing name (e.g. "benchy.gcode") and data is the raw file content.
	// Returns nil on success.
	UploadGCode(ctx context.Context, filename string, data []byte) error
	// StartPrint begins printing a file that has already been uploaded to
	// the printer. The filename should match what was passed to UploadGCode.
	StartPrint(ctx context.Context, filename string) error
}
