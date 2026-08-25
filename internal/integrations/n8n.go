package integrations

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// SendN8n posts a JSON payload to an n8n/Zapier webhook URL.
// The payload includes the event type and message, allowing the
// automation platform to filter/route based on the events field.
func SendN8n(cfg map[string]string, event, message string) error {
	webhook := cfg["webhook_url"]
	if webhook == "" {
		return fmt.Errorf("n8n: webhook_url is required")
	}

	events := cfg["events"]
	if events != "" {
		allowed := strings.Split(events, ",")
		matched := false
		for _, e := range allowed {
			if strings.TrimSpace(e) == event {
				matched = true
				break
			}
		}
		if !matched {
			return nil // event not in the configured list, skip silently
		}
	}

	body := map[string]string{
		"event":   event,
		"message": message,
	}
	b, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("n8n: marshal request: %w", err)
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Post(webhook, "application/json", bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("n8n: send: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("n8n: http %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}
