package ai

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ChatMessageForAPI is the format Gemini expects for each content part.
type ChatMessageForAPI struct {
	Role  string      `json:"role"`  // "user" or "model"
	Parts []ChatPart  `json:"parts"`
}

// ChatPart is either text or an inline image.
type ChatPart struct {
	Text       string `json:"text,omitempty"`
	InlineData *struct {
		MimeType string `json:"mimeType"`
		Data     string `json:"data"`
	} `json:"inlineData,omitempty"`
}

// ChatRequest is the full request to start or continue a chat.
type ChatRequest struct {
	APIKey    string             `json:"apiKey"`
	Messages  []ChatMessageForAPI `json:"messages"`
}

// ChatResponse is the result from Gemini.
type ChatResponse struct {
	Text string `json:"text"`
	Raw  string `json:"raw,omitempty"`
}

// Chat sends a multi-turn conversation to Gemini and returns the response.
// The full conversation history (including images) is sent each time since
// the Gemini REST API is stateless.
func Chat(req ChatRequest) (*ChatResponse, error) {
	if req.APIKey == "" {
		return nil, fmt.Errorf("API key required")
	}
	if len(req.Messages) == 0 {
		return nil, fmt.Errorf("no messages")
	}

	type geminiReq struct {
		Contents         []ChatMessageForAPI `json:"contents"`
		GenerationConfig struct {
			Temperature     float64 `json:"temperature"`
			MaxOutputTokens int     `json:"maxOutputTokens"`
		} `json:"generationConfig"`
	}

	body := geminiReq{
		Contents: req.Messages,
		GenerationConfig: struct {
			Temperature     float64 `json:"temperature"`
			MaxOutputTokens int     `json:"maxOutputTokens"`
		}{
			Temperature:     0.4,
			MaxOutputTokens: 2048,
		},
	}

	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.0-flash:generateContent?key=" + req.APIKey

	httpReq, err := http.NewRequest("POST", url, bytes.NewReader(bodyJSON))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("gemini request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("gemini API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var geminiResp struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(respBody, &geminiResp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	var text string
	if len(geminiResp.Candidates) > 0 && len(geminiResp.Candidates[0].Content.Parts) > 0 {
		text = geminiResp.Candidates[0].Content.Parts[0].Text
	}

	return &ChatResponse{
		Text: text,
		Raw:  string(respBody),
	}, nil
}

// EncodeImageBase64 reads raw image bytes and returns base64-encoded string.
func EncodeImageBase64(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}
