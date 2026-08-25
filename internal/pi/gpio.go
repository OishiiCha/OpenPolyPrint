package pi

import (
	"bufio"
	"embed"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

//go:embed all:scripts
var gpioScriptsFS embed.FS

// GPIO backend type
type gpioBackend int

const (
	backendNone     gpioBackend = iota
	backendPigs                 // pigpiod via `pigs` client
	backendLgpio                // lgpio via persistent Python daemon
	backendLibgpiod             // libgpiod via `gpioset`/`gpioget`
	backendSysfs                // /sys/class/gpio
)

// lgpioDaemon manages the persistent Python process for GPIO control.
type lgpioDaemon struct {
	cmd    *exec.Cmd
	stdin  *json.Encoder
	stdout *bufio.Scanner
	mu     sync.Mutex
}

// Manager controls GPIO pins. It tries multiple backends in order:
//  1. pigpio daemon (pigpiod) via the `pigs` client
//  2. lgpio via persistent Python daemon (python3-lgpio)
//  3. libgpiod via `gpioset`/`gpioget`
//  4. sysfs (/sys/class/gpio) — legacy fallback
type Manager struct {
	mu       sync.Mutex
	exported map[int]bool

	pigsAvailable     bool
	lgpioAvailable    bool
	libgpiodAvailable bool
	sysfsAvailable    bool
	checked           bool

	pythonCmd string
	scriptDir string
	lgpio     *lgpioDaemon
}

// NewManager creates a new GPIO Manager.
func NewManager() *Manager {
	g := &Manager{
		exported: make(map[int]bool),
	}
	g.checkBackends()
	return g
}

func (g *Manager) findPython() string {
	for _, name := range []string{"python3", "python"} {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	for _, path := range []string{"/usr/bin/python3", "/usr/local/bin/python3", "/usr/bin/python"} {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path
		}
	}
	return ""
}

func (g *Manager) extractLgpioScript() (string, error) {
	if g.scriptDir != "" {
		if _, err := os.Stat(filepath.Join(g.scriptDir, "gpio_lgpio.py")); err == nil {
			return filepath.Join(g.scriptDir, "gpio_lgpio.py"), nil
		}
	}

	tmpDir, err := os.MkdirTemp("", "gpio_lgpio_*")
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}

	data, err := gpioScriptsFS.ReadFile("scripts/gpio_lgpio.py")
	if err != nil {
		os.RemoveAll(tmpDir)
		return "", fmt.Errorf("read embedded script: %w", err)
	}

	scriptPath := filepath.Join(tmpDir, "gpio_lgpio.py")
	if err := os.WriteFile(scriptPath, data, 0644); err != nil {
		os.RemoveAll(tmpDir)
		return "", fmt.Errorf("write temp script: %w", err)
	}

	g.scriptDir = tmpDir
	return scriptPath, nil
}

// startLgpioDaemon starts the persistent Python lgpio daemon and verifies it's ready.
func (g *Manager) startLgpioDaemon() error {
	python := g.pythonCmd
	if python == "" {
		return fmt.Errorf("python not found")
	}
	scriptPath, err := g.extractLgpioScript()
	if err != nil {
		return err
	}

	cmd := exec.Command(python, scriptPath)
	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("create stdin pipe: %w", err)
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("create stdout pipe: %w", err)
	}
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start lgpio daemon: %w", err)
	}

	d := &lgpioDaemon{
		cmd:    cmd,
		stdin:  json.NewEncoder(stdinPipe),
		stdout: bufio.NewScanner(stdoutPipe),
	}

	// Read the ready message
	if !d.stdout.Scan() {
		errMsg := d.stdout.Text()
		if errMsg == "" {
			errMsg = "no response from daemon"
		}
		_ = cmd.Process.Kill()
		return fmt.Errorf("lgpio daemon startup: %s", errMsg)
	}

	var ready struct {
		OK    bool   `json:"ok"`
		Msg   string `json:"msg"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(d.stdout.Bytes(), &ready); err != nil {
		_ = cmd.Process.Kill()
		return fmt.Errorf("parse lgpio daemon response: %w", err)
	}
	if !ready.OK {
		_ = cmd.Process.Kill()
		return fmt.Errorf("lgpio daemon: %s", ready.Error)
	}

	g.lgpio = d
	return nil
}

// lgpioSend sends a command to the lgpio daemon and reads the response.
func (g *Manager) lgpioSend(req map[string]any) (map[string]any, error) {
	if g.lgpio == nil {
		return nil, fmt.Errorf("lgpio daemon not running")
	}
	g.lgpio.mu.Lock()
	defer g.lgpio.mu.Unlock()

	if err := g.lgpio.stdin.Encode(req); err != nil {
		return nil, fmt.Errorf("send to lgpio daemon: %w", err)
	}
	if !g.lgpio.stdout.Scan() {
		errMsg := g.lgpio.stdout.Text()
		if errMsg == "" {
			errMsg = "daemon closed connection"
		}
		return nil, fmt.Errorf("lgpio daemon: %s", errMsg)
	}

	var resp map[string]any
	if err := json.Unmarshal(g.lgpio.stdout.Bytes(), &resp); err != nil {
		return nil, fmt.Errorf("parse lgpio daemon response: %w", err)
	}
	return resp, nil
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

	// Check lgpio (Python) — start the daemon to verify it works
	if python := g.findPython(); python != "" {
		g.pythonCmd = python
		if err := g.startLgpioDaemon(); err == nil {
			g.lgpioAvailable = true
		} else {
			log.Printf("[pi] lgpio daemon failed to start: %v", err)
			if g.lgpio != nil {
				_ = g.lgpio.cmd.Process.Kill()
				g.lgpio = nil
			}
			if g.scriptDir != "" {
				os.RemoveAll(g.scriptDir)
				g.scriptDir = ""
			}
		}
	}

	// Check libgpiod (gpioset / gpioget)
	if _, err := exec.LookPath("gpioset"); err == nil {
		if _, err2 := exec.LookPath("gpioget"); err2 == nil {
			g.libgpiodAvailable = true
		}
	}

	// Check sysfs — verify the export file exists AND is openable for writing
	if f, err := os.OpenFile("/sys/class/gpio/export", os.O_WRONLY, 0); err == nil {
		f.Close()
		g.sysfsAvailable = true
	}

	g.checked = true

	backend := "none"
	if g.pigsAvailable {
		backend = "pigpiod (pigs)"
	} else if g.lgpioAvailable {
		backend = "lgpio (python daemon)"
	} else if g.libgpiodAvailable {
		backend = "libgpiod (gpioset/gpioget)"
	} else if g.sysfsAvailable {
		backend = "sysfs (/sys/class/gpio)"
	}
	log.Printf("[pi] gpio backend: %s (pigs=%v lgpio=%v libgpiod=%v sysfs=%v)",
		backend, g.pigsAvailable, g.lgpioAvailable, g.libgpiodAvailable, g.sysfsAvailable)
}

func (g *Manager) preferredBackend() gpioBackend {
	if !g.checked {
		g.checkBackends()
	}
	if g.pigsAvailable {
		return backendPigs
	}
	if g.lgpioAvailable {
		return backendLgpio
	}
	if g.libgpiodAvailable {
		return backendLibgpiod
	}
	if g.sysfsAvailable {
		return backendSysfs
	}
	return backendNone
}

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
			if g.lgpioAvailable {
				g.pigsAvailable = false
				return g.exportLgpio(pin)
			}
			return err
		}
		g.exported[pin] = true
		return nil
	case backendLgpio:
		return g.exportLgpio(pin)
	case backendLibgpiod:
		return g.exportLibgpiod(pin)
	case backendSysfs:
		return g.exportSysfs(pin)
	}
	return fmt.Errorf("no GPIO backend available (install pigpiod, python3-lgpio, or libgpiod; or mount /sys/class/gpio)")
}

func (g *Manager) exportLgpio(pin int) error {
	resp, err := g.lgpioSend(map[string]any{
		"cmd":  "mode",
		"pin":  pin,
		"mode": "w",
	})
	if err != nil {
		return fmt.Errorf("export gpio %d: %w", pin, err)
	}
	if ok, _ := resp["ok"].(bool); !ok {
		errMsg, _ := resp["error"].(string)
		return fmt.Errorf("export gpio %d: %s", pin, errMsg)
	}
	g.exported[pin] = true
	return nil
}

func (g *Manager) exportLibgpiod(pin int) error {
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
		g.sysfsAvailable = false
		return fmt.Errorf("export gpio %d: %w (sysfs unavailable — use pigpiod, lgpio, or libgpiod)", pin, err)
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
	case backendLgpio:
		mode := "r"
		if strings.HasPrefix(direction, "out") {
			mode = "w"
		}
		resp, err := g.lgpioSend(map[string]any{
			"cmd":  "mode",
			"pin":  pin,
			"mode": mode,
		})
		if err != nil {
			return err
		}
		if ok, _ := resp["ok"].(bool); !ok {
			errMsg, _ := resp["error"].(string)
			return fmt.Errorf("set direction gpio %d: %s", pin, errMsg)
		}
		return nil
	case backendLibgpiod:
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
	case backendLgpio:
		resp, err := g.lgpioSend(map[string]any{
			"cmd":   "write",
			"pin":   pin,
			"value": value,
		})
		if err != nil {
			return err
		}
		if ok, _ := resp["ok"].(bool); !ok {
			errMsg, _ := resp["error"].(string)
			return fmt.Errorf("write gpio %d: %s", pin, errMsg)
		}
		return nil
	case backendLibgpiod:
		chip := findGpiochip()
		cmd := exec.Command("gpioset", chip, fmt.Sprintf("%d=%d", pin, value))
		if err := cmd.Start(); err != nil {
			return fmt.Errorf("gpioset gpio %d: %w", pin, err)
		}
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
	case backendLgpio:
		resp, err := g.lgpioSend(map[string]any{
			"cmd": "read",
			"pin": pin,
		})
		if err != nil {
			return -1, fmt.Errorf("read gpio %d: %w", pin, err)
		}
		if ok, _ := resp["ok"].(bool); !ok {
			errMsg, _ := resp["error"].(string)
			return -1, fmt.Errorf("read gpio %d: %s", pin, errMsg)
		}
		val, _ := resp["value"].(float64)
		return int(val), nil
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
	switch backend {
	case backendLgpio:
		_, _ = g.lgpioSend(map[string]any{"cmd": "free", "pin": pin})
	case backendSysfs:
		_ = os.WriteFile("/sys/class/gpio/unexport", []byte(strconv.Itoa(pin)), 0o644)
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
	case backendLgpio:
		return "lgpio"
	case backendLibgpiod:
		return "libgpiod"
	case backendSysfs:
		return "sysfs"
	}
	return "none"
}

// Close stops the lgpio daemon if running.
func (g *Manager) Close() {
	if g.lgpio != nil {
		_, _ = g.lgpioSend(map[string]any{"cmd": "quit"})
		_ = g.lgpio.cmd.Process.Kill()
		_ = g.lgpio.cmd.Wait()
		g.lgpio = nil
	}
	if g.scriptDir != "" {
		os.RemoveAll(g.scriptDir)
		g.scriptDir = ""
	}
}
