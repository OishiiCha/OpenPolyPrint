package push

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	webpush "github.com/SherClockHolmes/webpush-go"
)

// Subscription represents a browser push subscription.
type Subscription struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256DH string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
}

// Manager handles Web Push subscriptions and sending notifications.
type Manager struct {
	mu            sync.Mutex
	subscriptions []Subscription
	path          string
	vapidPublic   string
	vapidPrivate  string
}

// NewManager creates a push manager. VAPID keys are loaded from disk or
// generated on first run.
func NewManager(settingsDir string) *Manager {
	m := &Manager{
		path: filepath.Join(settingsDir, "push_subscriptions.json"),
	}
	m.loadSubscriptions()
	m.loadOrGenerateKeys(settingsDir)
	return m
}

func (m *Manager) loadSubscriptions() {
	data, err := os.ReadFile(m.path)
	if err != nil {
		return
	}
	_ = json.Unmarshal(data, &m.subscriptions)
}

func (m *Manager) saveSubscriptions() {
	data, _ := json.MarshalIndent(m.subscriptions, "", "  ")
	_ = os.WriteFile(m.path, data, 0o600)
}

func (m *Manager) loadOrGenerateKeys(settingsDir string) {
	pubPath := filepath.Join(settingsDir, "vapid_public.key")
	privPath := filepath.Join(settingsDir, "vapid_private.key")

	if pub, err := os.ReadFile(pubPath); err == nil {
		if priv, err := os.ReadFile(privPath); err == nil {
			m.vapidPublic = string(pub)
			m.vapidPrivate = string(priv)
			return
		}
	}

	// Generate new keys
	priv, pub, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		return
	}
	m.vapidPublic = pub
	m.vapidPrivate = priv
	_ = os.WriteFile(pubPath, []byte(pub), 0o600)
	_ = os.WriteFile(privPath, []byte(priv), 0o600)
}

// VapidPublicKey returns the public key for the frontend.
func (m *Manager) VapidPublicKey() string {
	return m.vapidPublic
}

// AddSubscription adds a browser push subscription.
func (m *Manager) AddSubscription(sub Subscription) {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Check if already exists
	for _, existing := range m.subscriptions {
		if existing.Endpoint == sub.Endpoint {
			return
		}
	}
	m.subscriptions = append(m.subscriptions, sub)
	m.saveSubscriptions()
}

// RemoveSubscription removes a subscription by endpoint.
func (m *Manager) RemoveSubscription(endpoint string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, sub := range m.subscriptions {
		if sub.Endpoint == endpoint {
			m.subscriptions = append(m.subscriptions[:i], m.subscriptions[i+1:]...)
			m.saveSubscriptions()
			return
		}
	}
}

// Send sends a push notification to all subscribers.
func (m *Manager) Send(title, body string) {
	m.mu.Lock()
	subs := make([]Subscription, len(m.subscriptions))
	copy(subs, m.subscriptions)
	m.mu.Unlock()

	if m.vapidPrivate == "" || m.vapidPublic == "" {
		return
	}

	payload, _ := json.Marshal(map[string]string{
		"title": title,
		"body":  body,
		"icon":  "/icon-192.png",
		"badge": "/icon-192.png",
		"data":  "{\"url\":\"/\"}",
	})

	for _, sub := range subs {
		s := &webpush.Subscription{
			Endpoint: sub.Endpoint,
			Keys: webpush.Keys{
				P256dh: sub.Keys.P256DH,
				Auth:   sub.Keys.Auth,
			},
		}
		resp, err := webpush.SendNotification(payload, s, &webpush.Options{
			Subscriber:      "openpolyprint@local",
			VAPIDPublicKey:  m.vapidPublic,
			VAPIDPrivateKey: m.vapidPrivate,
			TTL:             60,
		})
		if err != nil {
			continue
		}
		_ = resp.Body.Close()
	}
}
