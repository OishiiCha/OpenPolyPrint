package integrations

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// SendDiscord posts a message to a Discord webhook.
func SendDiscord(cfg map[string]string, message string) error {
	webhook := cfg["webhook_url"]
	if webhook == "" {
		return fmt.Errorf("discord: webhook_url is required")
	}

	body := map[string]string{"content": message}
	b, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("discord: marshal request: %w", err)
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Post(webhook, "application/json", bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("discord: send: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("discord: http %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}
