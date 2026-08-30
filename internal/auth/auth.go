package auth

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Manager handles session-based authentication with a configurable passcode.
// When no passcode is set, authentication is disabled (open access).
type Manager struct {
	mu       sync.RWMutex
	passcode string
	sessions map[string]time.Time // token -> expiry
}

// NewManager creates a new auth manager with the given passcode.
// If passcode is empty, auth is disabled.
func NewManager(passcode string) *Manager {
	return &Manager{
		passcode: passcode,
		sessions: make(map[string]time.Time),
	}
}

// SetPasscode updates the passcode. Empty string disables auth.
func (m *Manager) SetPasscode(passcode string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.passcode = passcode
	if passcode == "" {
		// Clear all sessions when disabling
		m.sessions = make(map[string]time.Time)
	}
}

// Enabled returns true if authentication is required.
func (m *Manager) Enabled() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.passcode != ""
}

// Login validates the passcode and returns a session token.
func (m *Manager) Login(passcode string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.passcode == "" {
		return "", true // auth disabled, always succeeds
	}
	if passcode != m.passcode {
		return "", false
	}
	token := generateToken()
	m.sessions[token] = time.Now().Add(7 * 24 * time.Hour) // 7-day expiry
	return token, true
}

// Validate checks if a token is valid (exists and not expired).
func (m *Manager) Validate(token string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.passcode == "" {
		return true // auth disabled
	}
	expiry, ok := m.sessions[token]
	if !ok {
		return false
	}
	if time.Now().After(expiry) {
		return false
	}
	return true
}

// Logout removes a session token.
func (m *Manager) Logout(token string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, token)
}

// Cleanup removes expired sessions.
func (m *Manager) Cleanup() {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	for token, expiry := range m.sessions {
		if now.After(expiry) {
			delete(m.sessions, token)
		}
	}
}

// extractToken gets the session token from the request.
// Checks Authorization header first, then cookie.
func extractToken(r *http.Request) string {
	// Authorization: Bearer <token>
	if auth := r.Header.Get("Authorization"); auth != "" {
		if strings.HasPrefix(auth, "Bearer ") {
			return strings.TrimPrefix(auth, "Bearer ")
		}
		return auth
	}
	// Cookie
	if cookie, err := r.Cookie("openpolyprint_session"); err == nil {
		return cookie.Value
	}
	return ""
}

// Middleware wraps an http.Handler with authentication.
// Public paths are exempt from auth.
var publicPaths = map[string]bool{
	"/api/health":          true,
	"/api/auth/login":      true,
	"/api/auth/status":     true,
	"/api/version":         true, // OctoPrint compat
	"/api/connection":      true, // OctoPrint compat
	"/api/settings":        true, // OctoPrint compat
	"/api/login":           true, // OctoPrint compat (passive login)
	"/api/printerprofiles": true, // OctoPrint compat
	"/manifest.json":       true,
	"/sw.js":               true,
	"/favicon.ico":         true,
	"/logo.svg":            true,
	"/api/tls/ca":          true,
	"/api/tls/install/":    true,
}

// isPublicPath checks if a path should be exempt from authentication.
func isPublicPath(path string) bool {
	if publicPaths[path] {
		return true
	}
	// Allow OctoPrint file upload paths and subpaths (slicers need to work without auth)
	if path == "/api/files" || path == "/api/files/" || strings.HasPrefix(path, "/api/files/") {
		return true
	}
	// Allow OctoPrint compat endpoints that slicers probe
	if path == "/api/printer" || path == "/api/job" || path == "/api/timelapse" {
		return true
	}
	// Allow static assets (frontend needs to load before login)
	if !strings.HasPrefix(path, "/api/") {
		return true
	}
	// Allow TLS install script paths
	if strings.HasPrefix(path, "/api/tls/install/") {
		return true
	}
	return false
}

// Middleware returns an http.Handler that wraps the given handler with auth.
func (m *Manager) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !m.Enabled() || isPublicPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		token := extractToken(r)
		if m.Validate(token) {
			next.ServeHTTP(w, r)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
	})
}

func generateToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
