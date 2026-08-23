package integrations

import (
	"fmt"
	"sync"
)

// Manager holds the configured integrations and dispatches test/send calls.
type Manager struct {
	configs map[string]map[string]string
	mu      sync.RWMutex
}

// NewManager creates an empty integration manager.
func NewManager() *Manager {
	return &Manager{configs: make(map[string]map[string]string)}
}

// SetConfig stores the fields for an integration.
func (m *Manager) SetConfig(id string, cfg map[string]string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.configs[id] = cfg
}

// Config returns the stored config for an integration.
func (m *Manager) Config(id string) map[string]string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if cfg, ok := m.configs[id]; ok {
		return cfg
	}
	return nil
}

// Test sends a test message using an optional override configuration.
// If override is nil, the stored configuration is used.
func (m *Manager) Test(id, message string, override map[string]string) error {
	cfg := override
	if cfg == nil {
		cfg = m.Config(id)
	}
	if cfg == nil {
		return fmt.Errorf("no configuration for %s", id)
	}
	return m.sendWithConfig(id, cfg, message)
}

// Send dispatches a message through the given integration using stored config.
func (m *Manager) Send(id, message string) error {
	cfg := m.Config(id)
	if cfg == nil {
		return fmt.Errorf("no configuration for %s", id)
	}
	return m.sendWithConfig(id, cfg, message)
}

func (m *Manager) sendWithConfig(id string, cfg map[string]string, message string) error {
	switch id {
	case "telegram":
		return SendTelegram(cfg, message)
	case "discord":
		return SendDiscord(cfg, message)
	default:
		return fmt.Errorf("integration %s has no sender yet", id)
	}
}
