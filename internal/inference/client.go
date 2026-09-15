// Package inference talks to an OpenAI-compatible multimodal HTTP server.
// GameVision does not embed a local inference runtime.
package inference

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Config is the external VLM endpoint. All inference lives behind this HTTP API
// so 2B, 4B, and later models can be swapped without changing the agent.
type Config struct {
	BaseURL     string
	Model       string
	APIKey      string
	Timeout     time.Duration
	Temperature float64
	MaxTokens   int
}

// Client posts chat/completions requests with one PNG observation.
type Client struct {
	mu   sync.Mutex
	cfg  Config
	http *http.Client
}

func New(cfg Config) *Client {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 60 * time.Second
	}
	if cfg.MaxTokens <= 0 {
		cfg.MaxTokens = 16
	}
	return &Client{
		cfg: cfg,
		http: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
}

// Result is one model completion.
type Result struct {
	Text       string
	Latency    time.Duration
	StatusCode int
}

type chatRequest struct {
	Model       string        `json:"model"`
	Temperature float64       `json:"temperature"`
	MaxTokens   int           `json:"max_tokens"`
	CachePrompt bool          `json:"cache_prompt"`
	Messages    []chatMessage `json:"messages"`
}

type chatMessage struct {
	Role    string        `json:"role"`
	Content []contentPart `json:"content"`
}

type contentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *imageURL `json:"image_url,omitempty"`
}

type imageURL struct {
	URL string `json:"url"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content json.RawMessage `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// Complete sends prompt + PNG and returns the assistant text.
func (c *Client) Complete(ctx context.Context, prompt string, png []byte) (Result, error) {
	c.mu.Lock()
	baseURL := c.cfg.BaseURL
	model := c.cfg.Model
	apiKey := c.cfg.APIKey
	temp := c.cfg.Temperature
	maxTokens := c.cfg.MaxTokens
	c.mu.Unlock()
	if baseURL == "" {
		return Result{}, fmt.Errorf("inference: base_url is required")
	}
	if model == "" {
		return Result{}, fmt.Errorf("inference: model is required")
	}
	body, err := json.Marshal(chatRequest{
		Model:       model,
		Temperature: temp,
		MaxTokens:   maxTokens,
		CachePrompt: false, // llama.cpp prefix-cache skips a new screenshot
		Messages: []chatMessage{{
			Role: "user",
			Content: []contentPart{
				{
					Type: "image_url",
					ImageURL: &imageURL{
						URL: "data:image/png;base64," + base64.StdEncoding.EncodeToString(png),
					},
				},
				{Type: "text", Text: prompt},
			},
		}},
	})
	if err != nil {
		return Result{}, fmt.Errorf("inference: encode request: %w", err)
	}

	url := strings.TrimRight(baseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return Result{}, fmt.Errorf("inference: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	start := time.Now()
	resp, err := c.http.Do(req)
	latency := time.Since(start)
	if err != nil {
		return Result{Latency: latency}, fmt.Errorf("inference: POST %s: %w", url, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return Result{Latency: latency, StatusCode: resp.StatusCode}, fmt.Errorf("inference: read reply: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return Result{Latency: latency, StatusCode: resp.StatusCode}, fmt.Errorf("inference: HTTP %s: %s", resp.Status, snippet(data))
	}
	var cr chatResponse
	if err := json.Unmarshal(data, &cr); err != nil {
		return Result{Latency: latency, StatusCode: resp.StatusCode}, fmt.Errorf("inference: reply is not JSON: %w", err)
	}
	if len(cr.Choices) == 0 {
		return Result{Latency: latency, StatusCode: resp.StatusCode}, fmt.Errorf("inference: reply has no choices")
	}
	text, err := contentText(cr.Choices[0].Message.Content)
	if err != nil {
		return Result{Latency: latency, StatusCode: resp.StatusCode}, err
	}
	return Result{Text: text, Latency: latency, StatusCode: resp.StatusCode}, nil
}

func contentText(raw json.RawMessage) (string, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return "", nil
	}
	if raw[0] == '"' {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return "", fmt.Errorf("inference: content string: %w", err)
		}
		return s, nil
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parts); err != nil {
		return "", fmt.Errorf("inference: content parts: %w", err)
	}
	var b strings.Builder
	for _, p := range parts {
		if p.Type == "" || p.Type == "text" {
			b.WriteString(p.Text)
		}
	}
	return b.String(), nil
}

func snippet(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 240 {
		return s[:240] + "…"
	}
	return s
}
