package printers

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Manager owns the set of printer drivers and exposes their statuses.
type Manager struct {
	drivers []Driver
	mu      sync.RWMutex
}

// NewManager creates a manager from a slice of already configured drivers.
func NewManager(drivers []Driver) *Manager {
	return &Manager{drivers: drivers}
}

// ConnectAll tries to connect every driver concurrently.
func (m *Manager) ConnectAll(ctx context.Context) error {
	var wg sync.WaitGroup
	errs := make([]error, len(m.drivers))

	for i, d := range m.drivers {
		wg.Add(1)
		go func(i int, d Driver) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(ctx, 35*time.Second)
			defer cancel()
			if err := d.Connect(ctx); err != nil {
				errs[i] = fmt.Errorf("%s: %w", d.Name(), err)
			}
		}(i, d)
	}

	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

// DisconnectAll closes every driver connection.
func (m *Manager) DisconnectAll() error {
	var firstErr error
	for _, d := range m.drivers {
		if err := d.Disconnect(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// Statuses returns the latest status from every driver.
func (m *Manager) Statuses() []Status {
	m.mu.RLock()
	drivers := m.drivers
	m.mu.RUnlock()

	out := make([]Status, 0, len(drivers))
	for _, d := range drivers {
		s, err := d.Status()
		if err != nil {
			s = Status{
				ID:        d.PrinterID(),
				Name:      d.Name(),
				Type:      d.Type(),
				Online:    false,
				State:     "error",
				UpdatedAt: time.Now(),
				Error:     err.Error(),
			}
		}
		out = append(out, s)
	}
	return out
}

// Find returns the driver with the given printer ID.
func (m *Manager) Find(id string) Driver {
	for _, d := range m.drivers {
		if d.PrinterID() == id {
			return d
		}
	}
	return nil
}

// PausePrint sends a pause command to the requested printer.
func (m *Manager) PausePrint(ctx context.Context, id string) error {
	d := m.Find(id)
	if d == nil {
		return fmt.Errorf("printer not found: %s", id)
	}
	return d.PausePrint(ctx)
}

// StopPrint sends a stop command to the requested printer.
func (m *Manager) StopPrint(ctx context.Context, id string) error {
	d := m.Find(id)
	if d == nil {
		return fmt.Errorf("printer not found: %s", id)
	}
	return d.StopPrint(ctx)
}

// Home sends a home-all command to the requested printer.
func (m *Manager) Home(ctx context.Context, id string) error {
	d := m.Find(id)
	if d == nil {
		return fmt.Errorf("printer not found: %s", id)
	}
	return d.Home(ctx)
}

// Preheat sends a preheat command to the requested printer.
func (m *Manager) Preheat(ctx context.Context, id string, nozzle, bed float64) error {
	d := m.Find(id)
	if d == nil {
		return fmt.Errorf("printer not found: %s", id)
	}
	return d.Preheat(ctx, nozzle, bed)
}

// Cooldown sends a cooldown command to the requested printer.
func (m *Manager) Cooldown(ctx context.Context, id string) error {
	d := m.Find(id)
	if d == nil {
		return fmt.Errorf("printer not found: %s", id)
	}
	return d.Cooldown(ctx)
}

// AutoLevel sends an auto bed-leveling command to the requested printer.
func (m *Manager) AutoLevel(ctx context.Context, id string) error {
	d := m.Find(id)
	if d == nil {
		return fmt.Errorf("printer not found: %s", id)
	}
	return d.AutoLevel(ctx)
}

// SendGCode sends a raw G-code line to the requested printer.
func (m *Manager) SendGCode(ctx context.Context, id string, command string) error {
	d := m.Find(id)
	if d == nil {
		return fmt.Errorf("printer not found: %s", id)
	}
	return d.SendGCode(ctx, command)
}
