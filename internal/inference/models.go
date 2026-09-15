package inference

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ModelInfo is one llama.cpp-router (or OpenAI-compatible) catalog entry.
type ModelInfo struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Path   string `json:"path,omitempty"`
}

func (c *Client) Model() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cfg.Model
}

func (c *Client) SetModel(name string) {
	c.mu.Lock()
	c.cfg.Model = strings.TrimSpace(name)
	c.mu.Unlock()
}

// Origin is the inference server root with a trailing /v1 removed.
func (c *Client) Origin() string {
	c.mu.Lock()
	base := c.cfg.BaseURL
	c.mu.Unlock()
	return origin(base)
}

func origin(baseURL string) string {
	s := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	s = strings.TrimSuffix(s, "/v1")
	return strings.TrimRight(s, "/")
}

func (c *Client) apiKey() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cfg.APIKey
}

// ListModels returns discovered models. Prefers llama.cpp GET /models (includes
// load status). Falls back to OpenAI GET /v1/models.
func (c *Client) ListModels(ctx context.Context) ([]ModelInfo, error) {
	models, err := c.listRouter(ctx)
	if err == nil {
		return models, nil
	}
	fallback, ferr := c.listOpenAI(ctx)
	if ferr == nil {
		return fallback, nil
	}
	return nil, fmt.Errorf("inference: list models: %v", err)
}

func (c *Client) listRouter(ctx context.Context) ([]ModelInfo, error) {
	data, code, err := c.adminGET(ctx, c.Origin()+"/models")
	if err != nil {
		return nil, err
	}
	if code != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", code, snippet(data))
	}
	return parseModelList(data)
}

func (c *Client) listOpenAI(ctx context.Context) ([]ModelInfo, error) {
	base := strings.TrimRight(c.Origin(), "/") + "/v1/models"
	data, code, err := c.adminGET(ctx, base)
	if err != nil {
		return nil, err
	}
	if code != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", code, snippet(data))
	}
	return parseModelList(data)
}

// UnloadModel unloads one loaded model via POST /models/unload.
func (c *Client) UnloadModel(ctx context.Context, name string) error {
	id, err := c.resolve(ctx, name)
	if err != nil {
		return err
	}
	return c.postModel(ctx, "/models/unload", id)
}

// UnloadLoaded unloads every currently loaded/sleeping/loading model so VRAM
// is free before a replacement is loaded.
func (c *Client) UnloadLoaded(ctx context.Context) error {
	models, err := c.ListModels(ctx)
	if err != nil {
		return err
	}
	var first error
	for _, m := range models {
		if !occupiesVRAM(m.Status) {
			continue
		}
		if err := c.postModel(ctx, "/models/unload", m.ID); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// LoadModel loads one catalog model via POST /models/load.
func (c *Client) LoadModel(ctx context.Context, name string) error {
	id, err := c.resolve(ctx, name)
	if err != nil {
		return err
	}
	return c.postModel(ctx, "/models/load", id)
}

// Switch unloads whatever is currently in VRAM and loads name.
//
// llama.cpp router mode (POST /models/load) is used when that API exists.
// This machine's :8002 server is single-model llama-server, so switch
// restarts the process instead of looking name up in the currently-loaded list.
func (c *Client) Switch(ctx context.Context, name string) (ModelInfo, error) {
	if c.hasRouterLoad(ctx) {
		return c.switchRouter(ctx, name)
	}
	return hostSwap(ctx, c, name)
}

func (c *Client) switchRouter(ctx context.Context, name string) (ModelInfo, error) {
	id, err := c.resolve(ctx, name)
	if err != nil {
		return ModelInfo{}, err
	}
	if current, ok := c.loadedID(ctx, id); ok {
		c.SetModel(current.ID)
		return current, nil
	}
	if err := c.UnloadLoaded(ctx); err != nil {
		return ModelInfo{}, fmt.Errorf("inference: unload current model: %w", err)
	}
	if err := c.postModel(ctx, "/models/load", id); err != nil {
		return ModelInfo{}, fmt.Errorf("inference: load %s: %w", id, err)
	}
	deadline, ok := ctx.Deadline()
	wait := 3 * time.Minute
	if ok {
		wait = time.Until(deadline)
	}
	info, err := c.waitLoaded(ctx, id, wait)
	if err != nil {
		return ModelInfo{}, err
	}
	c.SetModel(info.ID)
	return info, nil
}

// HasRouterLoad is true when POST /models/load exists (llama.cpp router mode).
func (c *Client) HasRouterLoad(ctx context.Context) bool {
	return c.hasRouterLoad(ctx)
}

func (c *Client) hasRouterLoad(ctx context.Context) bool {
	_, code, err := c.adminPOST(ctx, c.Origin()+"/models/load", []byte(`{"model":""}`))
	if err != nil {
		return false
	}
	return code != http.StatusNotFound
}

func (c *Client) loadedID(ctx context.Context, id string) (ModelInfo, bool) {
	models, err := c.ListModels(ctx)
	if err != nil {
		return ModelInfo{}, false
	}
	for _, m := range models {
		if m.ID == id && strings.EqualFold(m.Status, "loaded") {
			return m, true
		}
	}
	return ModelInfo{}, false
}

func (c *Client) waitLoaded(ctx context.Context, id string, wait time.Duration) (ModelInfo, error) {
	deadline := time.Now().Add(wait)
	var last ModelInfo
	for {
		if err := ctx.Err(); err != nil {
			return last, err
		}
		models, err := c.ListModels(ctx)
		if err == nil {
			for _, m := range models {
				if m.ID != id {
					continue
				}
				last = m
				if strings.EqualFold(m.Status, "loaded") {
					return m, nil
				}
			}
		}
		if time.Now().After(deadline) {
			if last.ID == "" {
				return last, fmt.Errorf("inference: timed out waiting for %s to load", id)
			}
			return last, fmt.Errorf("inference: timed out waiting for %s (status %s)", id, last.Status)
		}
		select {
		case <-ctx.Done():
			return last, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func (c *Client) resolve(ctx context.Context, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("inference: model name is required")
	}
	models, err := c.ListModels(ctx)
	if err != nil {
		// Server may still accept the raw name (single-model llama.cpp).
		return name, nil
	}
	if id, ok := matchModel(name, models); ok {
		return id, nil
	}
	return "", fmt.Errorf("inference: model %q is not in the server catalog", name)
}

func matchModel(name string, models []ModelInfo) (string, bool) {
	want := strings.ToLower(strings.TrimSpace(name))
	for _, m := range models {
		if strings.EqualFold(m.ID, name) {
			return m.ID, true
		}
	}
	var hits []string
	for _, m := range models {
		id := strings.ToLower(m.ID)
		if strings.Contains(id, want) || strings.Contains(want, id) {
			hits = append(hits, m.ID)
		}
	}
	if len(hits) == 1 {
		return hits[0], true
	}
	return "", false
}

func occupiesVRAM(status string) bool {
	switch strings.ToLower(status) {
	case "loaded", "loading", "sleeping":
		return true
	default:
		return false
	}
}

func (c *Client) postModel(ctx context.Context, path, id string) error {
	body, err := json.Marshal(map[string]string{"model": id})
	if err != nil {
		return err
	}
	data, code, err := c.adminPOST(ctx, c.Origin()+path, body)
	if err != nil {
		return err
	}
	if code == http.StatusNotFound {
		return fmt.Errorf("server has no %s (need llama.cpp router mode). Restart the inference process with the new model", path)
	}
	if code != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", code, snippet(data))
	}
	return nil
}

func (c *Client) adminGET(ctx context.Context, url string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, err
	}
	return c.adminDo(req)
}

func (c *Client) adminPOST(ctx context.Context, url string, body []byte) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	return c.adminDo(req)
}

func (c *Client) adminDo(req *http.Request) ([]byte, int, error) {
	if key := c.apiKey(); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	// Load/unload can exceed the chat timeout; bound by the request context.
	httpClient := &http.Client{}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	return data, resp.StatusCode, err
}

func parseModelList(data []byte) ([]ModelInfo, error) {
	var wrap struct {
		Data []json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(data, &wrap); err != nil {
		return nil, fmt.Errorf("decode model list: %w", err)
	}
	out := make([]ModelInfo, 0, len(wrap.Data))
	for _, raw := range wrap.Data {
		info, err := parseOneModel(raw)
		if err != nil {
			continue
		}
		if info.ID != "" {
			out = append(out, info)
		}
	}
	return out, nil
}

func parseOneModel(raw json.RawMessage) (ModelInfo, error) {
	var row struct {
		ID     string          `json:"id"`
		Path   string          `json:"path"`
		Status json.RawMessage `json:"status"`
	}
	if err := json.Unmarshal(raw, &row); err != nil {
		return ModelInfo{}, err
	}
	return ModelInfo{ID: row.ID, Path: row.Path, Status: parseStatus(row.Status)}, nil
}

func parseStatus(raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return ""
	}
	if raw[0] == '"' {
		var s string
		_ = json.Unmarshal(raw, &s)
		return s
	}
	var obj struct {
		Value  string `json:"value"`
		Loaded bool   `json:"loaded"`
	}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return ""
	}
	if obj.Value != "" {
		return obj.Value
	}
	if obj.Loaded {
		return "loaded"
	}
	return "unloaded"
}
