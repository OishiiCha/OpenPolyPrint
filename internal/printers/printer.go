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

	// Print progress details (populated when available, e.g. Anker MQTT)
	LayerNum     int     `json:"layerNum,omitempty"`
	LayerCount   int     `json:"layerCount,omitempty"`
	PrintSpeed   float64 `json:"printSpeed,omitempty"`   // mm/s
	UsedFilament float64 `json:"usedFilament,omitempty"` // mm of filament used
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
	// MoveAxis moves an axis by a relative distance at the given speed.
	// axis is "X", "Y", or "Z". distance is in mm (can be negative).
	// speed is in mm/min.
	MoveAxis(ctx context.Context, axis string, distance float64, speed float64) error
	// SetNozzleTemp sets the nozzle target temperature in °C.
	SetNozzleTemp(ctx context.Context, temp float64) error
	// SetBedTemp sets the bed target temperature in °C.
	SetBedTemp(ctx context.Context, temp float64) error
	// Extrude extrudes (positive) or retracts (negative) filament by
	// the given amount in mm at the given feedrate (mm/min).
	Extrude(ctx context.Context, amount float64, feedrate float64) error
	// UploadGCode sends a G-code file to the printer. The filename is the
	// user-facing name (e.g. "benchy.gcode") and data is the raw file content.
	// Returns nil on success.
	UploadGCode(ctx context.Context, filename string, data []byte) error
	// StartPrint begins printing a file that has already been uploaded to
	// the printer. The filename should match what was passed to UploadGCode.
	StartPrint(ctx context.Context, filename string) error
}
