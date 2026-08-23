package pi

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// Manager controls GPIO pins. It prefers the pigpio daemon (pigpiod)
// through the `pigs` client, which works in Docker and on modern kernels
// where the deprecated sysfs interface (/sys/class/gpio) is unavailable.
// sysfs remains as a fallback for bare-metal installs without pigpiod.
type Manager struct {
	mu       sync.Mutex
	usePigs  bool
	exported map[int]bool
}

// NewManager creates a new GPIO Manager.
func NewManager() *Manager {
	_, err := exec.LookPath("pigs")
	g := &Manager{
		usePigs:  err == nil,
		exported: make(map[int]bool),
	}
	log.Printf("[pi] gpio backend: pigpiod (pigs available: %v)", g.usePigs)
	return g
}

// pigsCmd runs `pigs <args...>` and returns an error including stderr.
func pigsCmd(args ...string) error {
	var stderr strings.Builder
	cmd := exec.Command("pigs", args...)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("pigs %s: %s", strings.Join(args, " "), msg)
	}
	return nil
}

// Export makes the pin available for control. With pigpiod this sets the pin
// mode to output (there is no export concept); with sysfs it writes to
// /sys/class/gpio/export.
func (g *Manager) Export(pin int) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.exported[pin] {
		return nil
	}

	if g.usePigs {
		// pigs mode tokens: "w" = output, "r" = input (see pigs docs).
		if err := pigsCmd("m", strconv.Itoa(pin), "w"); err != nil {
			return err
		}
		g.exported[pin] = true
		return nil
	}

	err := os.WriteFile("/sys/class/gpio/export", []byte(strconv.Itoa(pin)), 0o644)
	if err != nil {
		// Pin may already be exported — some kernels return "Device or resource busy"
		// others return "invalid argument". Check if the gpio directory exists.
		gpioDir := filepath.Join("/sys/class/gpio", "gpio"+strconv.Itoa(pin))
		if _, statErr := os.Stat(gpioDir); statErr == nil {
			g.exported[pin] = true
			return nil
		}
		return fmt.Errorf("export gpio %d: %w (sysfs unavailable — is pigpiod running?)", pin, err)
	}

	g.exported[pin] = true
	return nil
}

// SetDirection sets the GPIO pin direction ("out" or "in").
func (g *Manager) SetDirection(pin int, direction string) error {
	if g.usePigs {
		mode := "r"
		if strings.HasPrefix(direction, "out") {
			mode = "w"
		}
		return pigsCmd("m", strconv.Itoa(pin), mode)
	}
	dirPath := filepath.Join("/sys/class/gpio/gpio"+strconv.Itoa(pin), "direction")
	err := os.WriteFile(dirPath, []byte(direction), 0o644)
	if err != nil {
		return fmt.Errorf("set direction gpio %d: %w", pin, err)
	}
	return nil
}

// Write sets the GPIO pin value (0 = low, 1 = high).
func (g *Manager) Write(pin int, value int) error {
	if g.usePigs {
		return pigsCmd("w", strconv.Itoa(pin), strconv.Itoa(value))
	}
	valPath := filepath.Join("/sys/class/gpio/gpio"+strconv.Itoa(pin), "value")
	err := os.WriteFile(valPath, []byte(strconv.Itoa(value)), 0o644)
	if err != nil {
		return fmt.Errorf("write gpio %d: %w", pin, err)
	}
	return nil
}

// Read reads the GPIO pin value (0 or 1).
func (g *Manager) Read(pin int) (int, error) {
	if g.usePigs {
		out, err := exec.Command("pigs", "r", strconv.Itoa(pin)).Output()
		if err != nil {
			return -1, fmt.Errorf("read gpio %d: %w", pin, err)
		}
		val, err := strconv.Atoi(strings.TrimSpace(string(out)))
		if err != nil {
			return -1, fmt.Errorf("read gpio %d: unexpected output %q", pin, string(out))
		}
		return val, nil
	}
	valPath := filepath.Join("/sys/class/gpio/gpio"+strconv.Itoa(pin), "value")
	data, err := os.ReadFile(valPath)
	if err != nil {
		return -1, fmt.Errorf("read gpio %d: %w", pin, err)
	}
	return strconv.Atoi(strings.TrimSpace(string(data)))
}

// Unexport releases the pin. pigpiod has no export state to undo, so this
// only forgets the pin locally there; with sysfs it writes to unexport.
func (g *Manager) Unexport(pin int) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if !g.exported[pin] {
		return nil
	}
	delete(g.exported, pin)

	if g.usePigs {
		return nil
	}

	unexportPath := "/sys/class/gpio/unexport"
	err := os.WriteFile(unexportPath, []byte(strconv.Itoa(pin)), 0o644)
	if err != nil {
		return fmt.Errorf("unexport gpio %d: %w", pin, err)
	}
	return nil
}

// IsAvailable returns true if GPIO control is possible: the pigpio daemon
// answers, or the sysfs interface exists.
func (g *Manager) IsAvailable() bool {
	if g.usePigs {
		return exec.Command("pigs", "pigpv").Run() == nil
	}
	_, err := os.Stat("/sys/class/gpio/export")
	return err == nil
}
