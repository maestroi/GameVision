package inference

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestOriginStripsV1(t *testing.T) {
	c := New(Config{BaseURL: "http://localhost:8000/v1"})
	if got := c.Origin(); got != "http://localhost:8000" {
		t.Fatalf("origin %q", got)
	}
}

func TestParseModelListStatusObject(t *testing.T) {
	raw := []byte(`{"data":[{"id":"Qwen3-VL-2B-Instruct.gguf","status":{"value":"loaded"}},{"id":"Qwen3-VL-4B-Instruct.gguf","status":{"value":"unloaded"}}]}`)
	got, err := parseModelList(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Status != "loaded" || got[1].Status != "unloaded" {
		t.Fatalf("%+v", got)
	}
}

func TestSwitchUnloadsThenLoads(t *testing.T) {
	loaded := "Qwen3-VL-2B-Instruct.gguf"
	var unloads, loads []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/models"):
			status2, status4 := "unloaded", "unloaded"
			if loaded == "Qwen3-VL-2B-Instruct.gguf" {
				status2 = "loaded"
			}
			if loaded == "Qwen3-VL-4B-Instruct.gguf" {
				status4 = "loaded"
			}
			_, _ = w.Write([]byte(`{"data":[{"id":"Qwen3-VL-2B-Instruct.gguf","status":{"value":"` + status2 + `"}},{"id":"Qwen3-VL-4B-Instruct.gguf","status":{"value":"` + status4 + `"}}]}`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/models/unload"):
			body, _ := io.ReadAll(r.Body)
			var req struct {
				Model string `json:"model"`
			}
			_ = json.Unmarshal(body, &req)
			unloads = append(unloads, req.Model)
			if req.Model == loaded {
				loaded = ""
			}
			_, _ = w.Write([]byte(`{"success":true}`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/models/load"):
			body, _ := io.ReadAll(r.Body)
			var req struct {
				Model string `json:"model"`
			}
			_ = json.Unmarshal(body, &req)
			if req.Model == "" {
				http.Error(w, `{"error":"model required"}`, http.StatusBadRequest)
				return
			}
			loads = append(loads, req.Model)
			loaded = req.Model
			_, _ = w.Write([]byte(`{"success":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL + "/v1"})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	info, err := c.Switch(ctx, "Qwen3-VL-4B")
	if err != nil {
		t.Fatal(err)
	}
	if info.ID != "Qwen3-VL-4B-Instruct.gguf" || info.Status != "loaded" {
		t.Fatalf("%+v", info)
	}
	if len(unloads) != 1 || unloads[0] != "Qwen3-VL-2B-Instruct.gguf" {
		t.Fatalf("unloads %v", unloads)
	}
	if len(loads) != 1 || loads[0] != "Qwen3-VL-4B-Instruct.gguf" {
		t.Fatalf("loads %v", loads)
	}
	if c.Model() != "Qwen3-VL-4B-Instruct.gguf" {
		t.Fatalf("client model %q", c.Model())
	}
}

func TestSwitchHostWhenNoRouterLoad(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/models"):
			_, _ = w.Write([]byte(`{"data":[{"id":"qwen3.8-27b","status":{"value":"loaded"}}]}`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/models/load"):
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	called := false
	orig := hostSwap
	hostSwap = func(_ context.Context, c *Client, name string) (ModelInfo, error) {
		called = true
		if name != "Qwen3-VL-4B-Instruct" {
			t.Fatalf("host swap name %q", name)
		}
		c.SetModel(name)
		return ModelInfo{ID: name, Status: "loaded", Path: "Qwen/Qwen3-VL-4B-Instruct-GGUF:Q4_K_M"}, nil
	}
	defer func() { hostSwap = orig }()

	c := New(Config{BaseURL: srv.URL + "/v1"})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	info, err := c.Switch(ctx, "Qwen3-VL-4B-Instruct")
	if err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("expected host swap, not router catalog lookup")
	}
	if info.ID != "Qwen3-VL-4B-Instruct" || c.Model() != "Qwen3-VL-4B-Instruct" {
		t.Fatalf("%+v model=%q", info, c.Model())
	}
}

func TestMatchModelExactAndSubstring(t *testing.T) {
	models := []ModelInfo{{ID: "Qwen3-VL-2B-Instruct.gguf"}, {ID: "Qwen3-VL-4B-Instruct.gguf"}}
	id, ok := matchModel("Qwen3-VL-4B-Instruct.gguf", models)
	if !ok || id != "Qwen3-VL-4B-Instruct.gguf" {
		t.Fatalf("%s %v", id, ok)
	}
	id, ok = matchModel("4B-Instruct", models)
	if !ok || id != "Qwen3-VL-4B-Instruct.gguf" {
		t.Fatalf("%s %v", id, ok)
	}
	if _, ok := matchModel("Qwen3-VL", models); ok {
		t.Fatal("ambiguous name should not match")
	}
}
