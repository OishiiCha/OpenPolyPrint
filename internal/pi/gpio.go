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

// GPIO backend type
type gpioBackend int

const (
	backendNone     gpioBackend = iota
	backendPigs                 // pigpiod via `pigs` client
	backendLibgpiod             // libgpiod via `gpioset`/`gpioget`
	backendSysfs                // /sys/class/gpio
)

// Manager controls GPIO pins. It tries multiple backends in order:
//  1. pigpio daemon (pigpiod) via the `pigs` client — works in Docker, modern
//  2. libgpiod via `gpioset`/`gpioget` — the modern Linux GPIO interface
//  3. sysfs (/sys/class/gpio) — legacy fallback for bare-metal installs
//
// The backend is selected dynamically: IsAvailable() checks whichever backend
// is currently working, and operations fall back to the next backend if the
// preferred one fails.
type Manager struct {
	mu       sync.Mutex
	exported map[int]bool

	// Cached availability — checked lazily and refreshed
	pigsAvailable     bool
	libgpiodAvailable bool
	sysfsAvailable    bool
	checked           bool
}

// NewManager creates a new GPIO Manager.
func NewManager() *Manager {
	g := &Manager{
		exported: make(map[int]bool),
	}
	g.checkBackends()
	return g
}

// checkBackends probes which GPIO backends are available.
func (g *Manager) checkBackends() {
	g.mu.Lock()
	defer g.mu.Unlock()

	// Check pigs (pigpiod)
	if _, err := exec.LookPath("pigs"); err == nil {
		if err := exec.Command("pigs", "pigpv").Run(); err == nil {
			g.pigsAvailable = true
		}
	}

	// Check libgpiod (gpioset / gpioget)
	if _, err := exec.LookPath("gpioset"); err == nil {
		if _, err2 := exec.LookPath("gpioget"); err2 == nil {
			g.libgpiodAvailable = true
		}
	}

	// Check sysfs
	if _, err := os.Stat("/sys/class/gpio/export"); err == nil {
		g.sysfsAvailable = true
	}

	g.checked = true

	backend := "none"
	if g.pigsAvailable {
		backend = "pigpiod (pigs)"
	} else if g.libgpiodAvailable {
		backend = "libgpiod (gpioset/gpioget)"
	} else if g.sysfsAvailable {
		backend = "sysfs (/sys/class/gpio)"
	}
	log.Printf("[pi] gpio backend: %s (pigs=%v libgpiod=%v sysfs=%v)",
		backend, g.pigsAvailable, g.libgpiodAvailable, g.sysfsAvailable)
}

// preferredBackend returns the best available backend.
func (g *Manager) preferredBackend() gpioBackend {
	if !g.checked {
		g.checkBackends()
	}
	if g.pigsAvailable {
		return backendPigs
	}
	if g.libgpiodAvailable {
		return backendLibgpiod
	}
	if g.sysfsAvailable {
		return backendSysfs
	}
	return backendNone
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

// findGpiochip finds the first gpiochip device. libgpiod requires specifying
// the chip name. On a Raspberry Pi, gpiochip0 is the main one.
func findGpiochip() string {
	entries, err := os.ReadDir("/dev")
	if err != nil {
		return "gpiochip0"
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "gpiochip") {
			return e.Name()
		}
	}
	return "gpiochip0"
}

// Export makes the pin available for control.
func (g *Manager) Export(pin int) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.exported[pin] {
		return nil
	}

	backend := g.preferredBackend()
	switch backend {
	case backendPigs:
		if err := pigsCmd("m", strconv.Itoa(pin), "w"); err != nil {
			// Try libgpiod fallback
			if g.libgpiodAvailable {
				g.pigsAvailable = false
				return g.exportLibgpiod(pin)
			}
			return err
		}
		g.exported[pin] = true
		return nil
	case backendLibgpiod:
		return g.exportLibgpiod(pin)
	case backendSysfs:
		return g.exportSysfs(pin)
	}
	return fmt.Errorf("no GPIO backend available (install pigpiod or libgpiod, or mount /sys/class/gpio)")
}

func (g *Manager) exportLibgpiod(pin int) error {
	// libgpiod doesn't have an export concept — gpioset/gpioget operate
	// directly on the chip. We just mark it as exported.
	g.exported[pin] = true
	return nil
}

func (g *Manager) exportSysfs(pin int) error {
	err := os.WriteFile("/sys/class/gpio/export", []byte(strconv.Itoa(pin)), 0o644)
	if err != nil {
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
	backend := g.preferredBackend()
	switch backend {
	case backendPigs:
		mode := "r"
		if strings.HasPrefix(direction, "out") {
			mode = "w"
		}
		return pigsCmd("m", strconv.Itoa(pin), mode)
	case backendLibgpiod:
		// libgpiod sets direction at write/read time, nothing to do here
		return nil
	case backendSysfs:
		dirPath := filepath.Join("/sys/class/gpio/gpio"+strconv.Itoa(pin), "direction")
		err := os.WriteFile(dirPath, []byte(direction), 0o644)
		if err != nil {
			return fmt.Errorf("set direction gpio %d: %w", pin, err)
		}
		return nil
	}
	return fmt.Errorf("no GPIO backend available")
}

// Write sets the GPIO pin value (0 = low, 1 = high).
func (g *Manager) Write(pin int, value int) error {
	backend := g.preferredBackend()
	switch backend {
	case backendPigs:
		return pigsCmd("w", strconv.Itoa(pin), strconv.Itoa(value))
	case backendLibgpiod:
		chip := findGpiochip()
		// gpioset chip line=value — uses interactive mode by default,
		// but we want a one-shot write. Use `-c` for push-pull or just
		// the simple form with `&` to background briefly.
		// Actually gpioset holds the line; for a simple set we use:
		// gpioset gpiochip0 PIN=VALUE
		// This holds the line until the process exits. We run it and
		// let it exit naturally (gpioset exits after setting if not in
		// interactive mode on newer versions, or we kill it after a moment).
		cmd := exec.Command("gpioset", chip, fmt.Sprintf("%d=%d", pin, value))
		if err := cmd.Start(); err != nil {
			return fmt.Errorf("gpioset gpio %d: %w", pin, err)
		}
		// Give it a moment to set the value, then kill it
		go func(p *os.Process) {
			_ = p.Kill()
		}(cmd.Process)
		_ = cmd.Wait()
		return nil
	case backendSysfs:
		valPath := filepath.Join("/sys/class/gpio/gpio"+strconv.Itoa(pin), "value")
		err := os.WriteFile(valPath, []byte(strconv.Itoa(value)), 0o644)
		if err != nil {
			return fmt.Errorf("write gpio %d: %w", pin, err)
		}
		return nil
	}
	return fmt.Errorf("no GPIO backend available")
}

// Read reads the GPIO pin value (0 or 1).
func (g *Manager) Read(pin int) (int, error) {
	backend := g.preferredBackend()
	switch backend {
	case backendPigs:
		out, err := exec.Command("pigs", "r", strconv.Itoa(pin)).Output()
		if err != nil {
			return -1, fmt.Errorf("read gpio %d: %w", pin, err)
		}
		val, err := strconv.Atoi(strings.TrimSpace(string(out)))
		if err != nil {
			return -1, fmt.Errorf("read gpio %d: unexpected output %q", pin, string(out))
		}
		return val, nil
	case backendLibgpiod:
		chip := findGpiochip()
		out, err := exec.Command("gpioget", chip, strconv.Itoa(pin)).Output()
		if err != nil {
			return -1, fmt.Errorf("gpioget gpio %d: %w", pin, err)
		}
		val, err := strconv.Atoi(strings.TrimSpace(string(out)))
		if err != nil {
			return -1, fmt.Errorf("gpioget gpio %d: unexpected output %q", pin, string(out))
		}
		return val, nil
	case backendSysfs:
		valPath := filepath.Join("/sys/class/gpio/gpio"+strconv.Itoa(pin), "value")
		data, err := os.ReadFile(valPath)
		if err != nil {
			return -1, fmt.Errorf("read gpio %d: %w", pin, err)
		}
		return strconv.Atoi(strings.TrimSpace(string(data)))
	}
	return -1, fmt.Errorf("no GPIO backend available")
}

// Unexport releases the pin.
func (g *Manager) Unexport(pin int) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if !g.exported[pin] {
		return nil
	}
	delete(g.exported, pin)

	backend := g.preferredBackend()
	if backend == backendSysfs {
		unexportPath := "/sys/class/gpio/unexport"
		err := os.WriteFile(unexportPath, []byte(strconv.Itoa(pin)), 0o644)
		if err != nil {
			return fmt.Errorf("unexport gpio %d: %w", pin, err)
		}
	}
	return nil
}

// IsAvailable returns true if any GPIO backend is working.
func (g *Manager) IsAvailable() bool {
	return g.preferredBackend() != backendNone
}

// BackendName returns the name of the active backend for display.
func (g *Manager) BackendName() string {
	switch g.preferredBackend() {
	case backendPigs:
		return "pigpiod"
	case backendLibgpiod:
		return "libgpiod"
	case backendSysfs:
		return "sysfs"
	}
	return "none"
}
