package printers

import (
	"context"
	"fmt"
	"time"
)

// StaticDriver is a placeholder driver for printers whose protocol is not yet
// implemented. It stores the user-supplied configuration and reports Offline.
type StaticDriver struct {
	cfg PrinterConfig
}

var _ Driver = (*StaticDriver)(nil)

// NewStaticDriver creates a static/offline driver from a PrinterConfig.
func NewStaticDriver(cfg PrinterConfig) *StaticDriver {
	return &StaticDriver{cfg: cfg}
}

// PrinterID returns the configured printer ID.
func (d *StaticDriver) PrinterID() string { return d.cfg.ID }

// Name returns the configured printer name.
func (d *StaticDriver) Name() string { return d.cfg.Name }

// Type returns the configured printer type.
func (d *StaticDriver) Type() string { return d.cfg.Type }

// Connect is a no-op for the static driver.
func (d *StaticDriver) Connect(ctx context.Context) error { return nil }

// Disconnect is a no-op for the static driver.
func (d *StaticDriver) Disconnect() error { return nil }

// Status returns a permanent Offline status.
func (d *StaticDriver) Status() (Status, error) {
	return Status{
		ID:         d.cfg.ID,
		Name:       d.cfg.Name,
		Type:       d.cfg.Type,
		Online:     false,
		State:      "offline",
		StatusText: "Offline",
		Temps:      Temps{},
		Progress:   0,
		UpdatedAt:  time.Now(),
		Error:      fmt.Sprintf("%s driver not yet implemented", d.cfg.Type),
	}, nil
}

// PausePrint returns an error for the static driver.
func (d *StaticDriver) PausePrint(ctx context.Context) error {
	return fmt.Errorf("not implemented for %s", d.cfg.Type)
}

// StopPrint returns an error for the static driver.
func (d *StaticDriver) StopPrint(ctx context.Context) error {
	return fmt.Errorf("not implemented for %s", d.cfg.Type)
}

// Home returns an error for the static driver.
func (d *StaticDriver) Home(ctx context.Context) error {
	return fmt.Errorf("not implemented for %s", d.cfg.Type)
}

// Preheat returns an error for the static driver.
func (d *StaticDriver) Preheat(ctx context.Context, nozzle, bed float64) error {
	return fmt.Errorf("not implemented for %s", d.cfg.Type)
}

// Cooldown returns an error for the static driver.
func (d *StaticDriver) Cooldown(ctx context.Context) error {
	return fmt.Errorf("not implemented for %s", d.cfg.Type)
}

// AutoLevel returns an error for the static driver.
func (d *StaticDriver) AutoLevel(ctx context.Context) error {
	return fmt.Errorf("not implemented for %s", d.cfg.Type)
}

// SendGCode returns an error for the static driver.
func (d *StaticDriver) SendGCode(ctx context.Context, command string) error {
	return fmt.Errorf("not implemented for %s", d.cfg.Type)
}
