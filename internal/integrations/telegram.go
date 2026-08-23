package integrations

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// SendTelegram sends a text message using the Telegram Bot API.
func SendTelegram(cfg map[string]string, message string) error {
	token := cfg["token"]
	chatID := cfg["chat_id"]
	if token == "" || chatID == "" {
		return fmt.Errorf("telegram: token and chat_id are required")
	}

	u := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", url.PathEscape(token))
	body := map[string]string{
		"chat_id": chatID,
		"text":    message,
	}
	b, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("telegram: marshal request: %w", err)
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Post(u, "application/json", bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("telegram: send: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("telegram: http %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}
