package ai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// geminiHTTPClient is a shared HTTP client configured for Gemini API calls.
// It forces IPv4 to avoid IPv6 connection reset issues, and has a generous
// timeout for large requests (gcode + images).
var geminiHTTPClient = &http.Client{
	Timeout: 120 * time.Second,
	Transport: &http.Transport{
		// Force IPv4 — IPv6 connectivity to Google APIs can be unreliable
		// on some networks, causing "connection reset by peer" errors.
		DialContext:           dialContextIPv4,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          10,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   15 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	},
}

// dialContextIPv4 is a DialContext that only uses IPv4, avoiding IPv6
// connection reset issues on networks with broken IPv6 routing.
func dialContextIPv4(ctx context.Context, network, addr string) (net.Conn, error) {
	d := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}
	return d.DialContext(ctx, "tcp4", addr)
}

// doGeminiRequest sends the request to Gemini with retry logic for transient
// network errors (connection reset, EOF, timeout).
func doGeminiRequest(url string, bodyJSON []byte) (*http.Response, error) {
	maxRetries := 3
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * 2 * time.Second)
		}
		httpReq, err := http.NewRequest("POST", url, bytes.NewReader(bodyJSON))
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}
		httpReq.Header.Set("Content-Type", "application/json")

		resp, err := geminiHTTPClient.Do(httpReq)
		if err != nil {
			lastErr = err
			// Retry on network errors (connection reset, EOF, timeout, etc.)
			if isTransientError(err) {
				continue
			}
			return nil, fmt.Errorf("gemini request: %w", err)
		}
		return resp, nil
	}
	return nil, fmt.Errorf("gemini request (after %d retries): %w", maxRetries, lastErr)
}

// isTransientError reports whether the error is likely a transient network
// issue worth retrying (connection reset, broken pipe, EOF, timeout).
func isTransientError(err error) bool {
	s := err.Error()
	return strings.Contains(s, "connection reset") ||
		strings.Contains(s, "broken pipe") ||
		strings.Contains(s, "EOF") ||
		strings.Contains(s, "timeout") ||
		strings.Contains(s, "no route to host") ||
		strings.Contains(s, "network is unreachable") ||
		strings.Contains(s, "temporarily unavailable")
}

// ChatMessageForAPI is the format Gemini expects for each content part.
type ChatMessageForAPI struct {
	Role  string     `json:"role"` // "user" or "model"
	Parts []ChatPart `json:"parts"`
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
	APIKey   string              `json:"apiKey"`
	Messages []ChatMessageForAPI `json:"messages"`
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
			ThinkingConfig  struct {
				ThinkingLevel string `json:"thinkingLevel"`
			} `json:"thinkingConfig"`
		} `json:"generationConfig"`
	}

	body := geminiReq{
		Contents: req.Messages,
		GenerationConfig: struct {
			Temperature     float64 `json:"temperature"`
			MaxOutputTokens int     `json:"maxOutputTokens"`
			ThinkingConfig  struct {
				ThinkingLevel string `json:"thinkingLevel"`
			} `json:"thinkingConfig"`
		}{
			Temperature:     0.4,
			MaxOutputTokens: 8192,
			ThinkingConfig: struct {
				ThinkingLevel string `json:"thinkingLevel"`
			}{
				ThinkingLevel: "minimal", // Minimal thinking for faster responses
			},
		},
	}

	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := "https://generativelanguage.googleapis.com/v1beta/models/gemini-3.6-flash:generateContent?key=" + req.APIKey

	resp, err := doGeminiRequest(url, bodyJSON)
	if err != nil {
		return nil, err
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
					Text    string `json:"text"`
					Thought bool   `json:"thought,omitempty"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(respBody, &geminiResp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	// Collect all non-thought text parts (thinking models may return
	// thought parts with thought=true that we should skip)
	var textParts []string
	if len(geminiResp.Candidates) > 0 {
		for _, part := range geminiResp.Candidates[0].Content.Parts {
			if part.Text != "" && !part.Thought {
				textParts = append(textParts, part.Text)
			}
		}
	}
	text := strings.Join(textParts, "\n")

	return &ChatResponse{
		Text: text,
		Raw:  string(respBody),
	}, nil
}

// EncodeImageBase64 reads raw image bytes and returns base64-encoded string.
func EncodeImageBase64(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}

// ProfileSuggestion is a single AI-suggested edit for a profile setting.
type ProfileSuggestion struct {
	Key            string `json:"key"`
	CurrentValue   string `json:"currentValue"`
	SuggestedValue string `json:"suggestedValue"`
	Reason         string `json:"reason"`
	Category       string `json:"category"` // "print", "filament", "printer"
}

// SuggestProfileEdits asks Gemini to analyze a profile and return structured
// suggestions for improving settings. Returns a list of specific setting changes.
func SuggestProfileEdits(apiKey, profileContent, profileName, profileType string) ([]ProfileSuggestion, string, error) {
	if apiKey == "" {
		return nil, "", fmt.Errorf("API key required")
	}

	systemPrompt := fmt.Sprintf(`You are a 3D printing expert. Analyze the following %s slicer profile named "%s" and suggest specific setting changes to improve print quality, speed, reliability, or material performance.

Return your response as a JSON array of suggested edits. Each edit must have:
- "key": the exact setting key from the INI file
- "currentValue": the current value as a string
- "suggestedValue": your suggested new value as a string
- "reason": a brief explanation of why this change is recommended
- "category": "print", "filament", or "printer"

Only suggest changes that are meaningful improvements. Do not suggest changes to meta keys like "inherits", "from", "version", "print_settings_id", etc.
If the profile is already well-optimized, return an empty array [].

Respond with ONLY the JSON array, no markdown formatting or explanation.

Profile content:
%s`, profileType, profileName, profileContent)

	type geminiReq struct {
		Contents         []ChatMessageForAPI `json:"contents"`
		GenerationConfig struct {
			Temperature      float64 `json:"temperature"`
			MaxOutputTokens  int     `json:"maxOutputTokens"`
			ResponseMimeType string  `json:"responseMimeType"`
			ThinkingConfig   struct {
				ThinkingLevel string `json:"thinkingLevel"`
			} `json:"thinkingConfig"`
		} `json:"generationConfig"`
	}

	body := geminiReq{
		Contents: []ChatMessageForAPI{
			{
				Role:  "user",
				Parts: []ChatPart{{Text: systemPrompt}},
			},
		},
		GenerationConfig: struct {
			Temperature      float64 `json:"temperature"`
			MaxOutputTokens  int     `json:"maxOutputTokens"`
			ResponseMimeType string  `json:"responseMimeType"`
			ThinkingConfig   struct {
				ThinkingLevel string `json:"thinkingLevel"`
			} `json:"thinkingConfig"`
		}{
			Temperature:      0.4,
			MaxOutputTokens:  8192,
			ResponseMimeType: "application/json",
			ThinkingConfig: struct {
				ThinkingLevel string `json:"thinkingLevel"`
			}{
				ThinkingLevel: "minimal",
			},
		},
	}

	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return nil, "", fmt.Errorf("marshal request: %w", err)
	}

	url := "https://generativelanguage.googleapis.com/v1beta/models/gemini-3.6-flash:generateContent?key=" + apiKey

	resp, err := doGeminiRequest(url, bodyJSON)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, "", fmt.Errorf("gemini API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	// Parse Gemini response
	var geminiResp struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text    string `json:"text"`
					Thought bool   `json:"thought,omitempty"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(respBody, &geminiResp); err != nil {
		return nil, "", fmt.Errorf("parse response: %w", err)
	}

	var textParts []string
	if len(geminiResp.Candidates) > 0 {
		for _, part := range geminiResp.Candidates[0].Content.Parts {
			if part.Text != "" && !part.Thought {
				textParts = append(textParts, part.Text)
			}
		}
	}
	fullText := strings.Join(textParts, "\n")

	// Parse the JSON suggestions
	// Strip any markdown code fences if present
	jsonText := fullText
	jsonText = strings.TrimSpace(jsonText)
	if strings.HasPrefix(jsonText, "```") {
		// Remove ```json or ``` prefix and trailing ```
		lines := strings.Split(jsonText, "\n")
		var cleaned []string
		for _, line := range lines {
			if strings.HasPrefix(line, "```") {
				continue
			}
			cleaned = append(cleaned, line)
		}
		jsonText = strings.Join(cleaned, "\n")
	}

	var suggestions []ProfileSuggestion
	if err := json.Unmarshal([]byte(jsonText), &suggestions); err != nil {
		// Return the raw text so the user can see what AI said
		return nil, fullText, fmt.Errorf("failed to parse AI suggestions as JSON: %w", err)
	}

	return suggestions, fullText, nil
}
