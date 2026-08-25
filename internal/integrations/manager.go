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
	case "n8n":
		return SendN8n(cfg, "message", message)
	default:
		return fmt.Errorf("integration %s has no sender yet", id)
	}
}

// SendEvent dispatches an event (e.g. "start", "complete", "fail") to all
// integrations that are configured. Telegram and Discord receive a text
// message; n8n receives a JSON payload with the event type so it can filter.
func (m *Manager) SendEvent(event, message string) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for id, cfg := range m.configs {
		if cfg == nil {
			continue
		}
		switch id {
		case "telegram":
			if cfg["token"] != "" && cfg["chat_id"] != "" {
				if err := SendTelegram(cfg, message); err != nil {
					fmt.Printf("[integration] telegram send failed: %v\n", err)
				}
			}
		case "discord":
			if cfg["webhook_url"] != "" {
				if err := SendDiscord(cfg, message); err != nil {
					fmt.Printf("[integration] discord send failed: %v\n", err)
				}
			}
		case "n8n":
			if cfg["webhook_url"] != "" {
				if err := SendN8n(cfg, event, message); err != nil {
					fmt.Printf("[integration] n8n send failed: %v\n", err)
				}
			}
		}
	}
}
