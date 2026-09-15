package pokemonred

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/maestroi/gamevision/internal/agent"
	"github.com/maestroi/gamevision/internal/inference"
	"github.com/maestroi/gamevision/internal/runtime"
)

func TestClosedLoopFramebufferToButton(t *testing.T) {
	var (
		mu       sync.Mutex
		sawImage bool
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []struct {
				Content []struct {
					Type     string `json:"type"`
					ImageURL *struct {
						URL string `json:"url"`
					} `json:"image_url"`
				} `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if len(body.Messages) > 0 {
			for _, p := range body.Messages[0].Content {
				if p.Type == "image_url" && p.ImageURL != nil && p.ImageURL.URL != "" {
					mu.Lock()
					sawImage = true
					mu.Unlock()
				}
			}
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"action\":\"RIGHT\"}"}}]}`))
	}))
	defer srv.Close()

	g, err := Open(Config{ROMBytes: testROM(), Scale: 2, HoldFrames: 1, SettleFrames: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	before := g.FrameCount()

	client := inference.New(inference.Config{BaseURL: srv.URL, Model: "test", Timeout: 2 * time.Second})
	rt := runtime.New(g, agent.New(client), nil, runtime.Config{
		Goal:         "move",
		Model:        "test",
		MaxDecisions: 3,
		History:      4,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	rt.Start(ctx)
	rt.Wait()

	snap := rt.Snapshot()
	if snap.Decision != 3 {
		t.Fatalf("decisions %d", snap.Decision)
	}
	if snap.LastAction != ActionRight {
		t.Fatalf("last action %q", snap.LastAction)
	}
	mu.Lock()
	okImg := sawImage
	mu.Unlock()
	if !okImg {
		t.Fatal("model server never received an image")
	}
	if g.FrameCount() <= before {
		t.Fatal("emulator did not advance")
	}
}
