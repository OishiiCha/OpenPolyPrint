package printers

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

// Manager owns the set of printer drivers and exposes their statuses.
type Manager struct {
	drivers []Driver
	mu      sync.RWMutex
	stopCh  chan struct{}
}

// NewManager creates a manager from a slice of already configured drivers.
func NewManager(drivers []Driver) *Manager {
	return &Manager{drivers: drivers, stopCh: make(chan struct{})}
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

// Watchdog starts a background goroutine that periodically checks if any
// driver is offline and attempts to reconnect it. This handles dropped
// PPPP/MQTT connections without requiring a full restart.
func (m *Manager) Watchdog(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-m.stopCh:
				return
			case <-ticker.C:
				m.reconnectOffline(ctx)
			}
		}
	}()
}

// StopWatchdog stops the reconnection watchdog.
func (m *Manager) StopWatchdog() {
	select {
	case <-m.stopCh:
		// already closed
	default:
		close(m.stopCh)
	}
}

// reconnectOffline attempts to reconnect any driver whose Status() reports
// it is not online.
func (m *Manager) reconnectOffline(ctx context.Context) {
	m.mu.RLock()
	drivers := m.drivers
	m.mu.RUnlock()

	for _, d := range drivers {
		s, err := d.Status()
		if err != nil || !s.Online {
			name := d.Name()
			log.Printf("[watchdog] %s appears offline, attempting reconnect...", name)
			// Disconnect first to clean up any stale state
			_ = d.Disconnect()
			reconnCtx, cancel := context.WithTimeout(ctx, 35*time.Second)
			if err := d.Connect(reconnCtx); err != nil {
				log.Printf("[watchdog] %s reconnect failed: %v", name, err)
			} else {
				log.Printf("[watchdog] %s reconnected successfully", name)
			}
			cancel()
		}
	}
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

// MoveAxis moves an axis by a relative distance on the requested printer.
func (m *Manager) MoveAxis(ctx context.Context, id string, axis string, distance float64, speed float64) error {
	d := m.Find(id)
	if d == nil {
		return fmt.Errorf("printer not found: %s", id)
	}
	return d.MoveAxis(ctx, axis, distance, speed)
}

// SetNozzleTemp sets the nozzle target temperature on the requested printer.
func (m *Manager) SetNozzleTemp(ctx context.Context, id string, temp float64) error {
	d := m.Find(id)
	if d == nil {
		return fmt.Errorf("printer not found: %s", id)
	}
	return d.SetNozzleTemp(ctx, temp)
}

// SetBedTemp sets the bed target temperature on the requested printer.
func (m *Manager) SetBedTemp(ctx context.Context, id string, temp float64) error {
	d := m.Find(id)
	if d == nil {
		return fmt.Errorf("printer not found: %s", id)
	}
	return d.SetBedTemp(ctx, temp)
}

// Extrude extrudes or retracts filament on the requested printer.
func (m *Manager) Extrude(ctx context.Context, id string, amount float64, feedrate float64) error {
	d := m.Find(id)
	if d == nil {
		return fmt.Errorf("printer not found: %s", id)
	}
	return d.Extrude(ctx, amount, feedrate)
}

// UploadGCode sends a G-code file to the requested printer.
func (m *Manager) UploadGCode(ctx context.Context, id string, filename string, data []byte) error {
	d := m.Find(id)
	if d == nil {
		return fmt.Errorf("printer not found: %s", id)
	}
	return d.UploadGCode(ctx, filename, data)
}

// StartPrint begins printing a previously uploaded file on the requested printer.
func (m *Manager) StartPrint(ctx context.Context, id string, filename string) error {
	d := m.Find(id)
	if d == nil {
		return fmt.Errorf("printer not found: %s", id)
	}
	return d.StartPrint(ctx, filename)
}

// FindByName returns the driver with the given printer name (case-insensitive).
func (m *Manager) FindByName(name string) Driver {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, d := range m.drivers {
		if strings.EqualFold(d.Name(), name) {
			return d
		}
	}
	return nil
}

// Drivers returns the list of all drivers.
func (m *Manager) Drivers() []Driver {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Driver, len(m.drivers))
	copy(out, m.drivers)
	return out
}

// Add appends a new driver to the manager.
func (m *Manager) Add(d Driver) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.drivers = append(m.drivers, d)
}

// Remove deletes a driver by ID and returns whether it was found.
func (m *Manager) Remove(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, d := range m.drivers {
		if d.PrinterID() == id {
			m.drivers = append(m.drivers[:i], m.drivers[i+1:]...)
			return true
		}
	}
	return false
}
