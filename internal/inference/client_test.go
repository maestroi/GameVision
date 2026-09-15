package inference

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCompleteSendsImageAndText(t *testing.T) {
	var gotBody chatRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Errorf("auth %q", r.Header.Get("Authorization"))
		}
		data, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(data, &gotBody); err != nil {
			t.Errorf("unmarshal: %v", err)
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"action\":\"RIGHT\"}"}}]}`))
	}))
	defer srv.Close()

	c := New(Config{
		BaseURL: srv.URL,
		Model:   "Qwen3-VL-2B-Instruct",
		APIKey:  "secret",
	})
	res, err := c.Complete(context.Background(), "choose", []byte("png"))
	if err != nil {
		t.Fatal(err)
	}
	if res.Text != `{"action":"RIGHT"}` {
		t.Fatalf("text %q", res.Text)
	}
	if gotBody.Model != "Qwen3-VL-2B-Instruct" {
		t.Fatalf("model %q", gotBody.Model)
	}
	if gotBody.Temperature != 0 {
		t.Fatalf("temperature %v", gotBody.Temperature)
	}
	if len(gotBody.Messages) != 1 || len(gotBody.Messages[0].Content) != 2 {
		t.Fatalf("content parts: %+v", gotBody.Messages)
	}
	if gotBody.Messages[0].Content[0].Type != "image_url" {
		t.Fatal("expected image first")
	}
	if !strings.HasPrefix(gotBody.Messages[0].Content[0].ImageURL.URL, "data:image/png;base64,") {
		t.Fatalf("image url %q", gotBody.Messages[0].Content[0].ImageURL.URL)
	}
	if gotBody.Messages[0].Content[1].Text != "choose" {
		t.Fatalf("prompt %q", gotBody.Messages[0].Content[1].Text)
	}
	if gotBody.CachePrompt {
		t.Fatal("cache_prompt must be false so each frame is actually seen")
	}
}

func TestCompleteAcceptsContentParts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":[{"type":"text","text":"UP"}]}}]}`))
	}))
	defer srv.Close()
	c := New(Config{BaseURL: srv.URL, Model: "m"})
	res, err := c.Complete(context.Background(), "p", []byte("x"))
	if err != nil {
		t.Fatal(err)
	}
	if res.Text != "UP" {
		t.Fatalf("text %q", res.Text)
	}
}

func TestCompleteHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusBadGateway)
	}))
	defer srv.Close()
	c := New(Config{BaseURL: srv.URL, Model: "m"})
	_, err := c.Complete(context.Background(), "p", []byte("x"))
	if err == nil {
		t.Fatal("expected error")
	}
}
