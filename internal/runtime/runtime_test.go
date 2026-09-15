package runtime

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/maestroi/gamevision/internal/agent"
	"github.com/maestroi/gamevision/pkg/game"
)

type fakeGame struct {
	mu      sync.Mutex
	frame   uint64
	applied []string
}

func (f *fakeGame) Name() string { return "Fake" }
func (f *fakeGame) Actions() []game.Action {
	return []game.Action{{Name: "A"}, {Name: "WAIT"}}
}
func (f *fakeGame) Observe(ctx context.Context) (game.Observation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.frame++
	return game.Observation{Image: []byte("png"), Width: 2, Height: 2, Frame: f.frame}, nil
}
func (f *fakeGame) Apply(ctx context.Context, action game.Action) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.applied = append(f.applied, action.Name)
	return nil
}
func (f *fakeGame) Close() error { return nil }
func (f *fakeGame) PreviewPNG() ([]byte, error) {
	return []byte("png"), nil
}

type fakeAgent struct {
	n int
}

func (a *fakeAgent) Decide(ctx context.Context, req agent.DecisionRequest) (agent.Decision, error) {
	a.n++
	name := "A"
	if a.n%2 == 0 {
		name = "WAIT"
	}
	return agent.Decision{Action: game.Action{Name: name}, Raw: name}, nil
}

func TestRuntimeMakesSuccessiveDecisions(t *testing.T) {
	g := &fakeGame{}
	r := New(g, &fakeAgent{}, nil, Config{Goal: "go", Model: "test", MaxDecisions: 4, History: 4})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	r.Start(ctx)
	r.Wait()
	snap := r.Snapshot()
	if snap.Decision != 4 {
		t.Fatalf("decisions %d", snap.Decision)
	}
	g.mu.Lock()
	n := len(g.applied)
	g.mu.Unlock()
	if n != 4 {
		t.Fatalf("applied %d", n)
	}
}

func TestHumanInputIsLoggedSeparately(t *testing.T) {
	g := &fakeGame{}
	r := New(g, &fakeAgent{}, nil, Config{Paused: true, History: 4})
	if err := r.Human(context.Background(), game.Action{Name: "A"}); err != nil {
		t.Fatal(err)
	}
	snap := r.Snapshot()
	if snap.LastSource != string(game.SourceHuman) || snap.LastAction != "A" {
		t.Fatalf("%+v", snap)
	}
	if snap.Decision != 0 {
		t.Fatalf("human input counted as decision %d", snap.Decision)
	}
}

type captureAgent struct {
	mu   sync.Mutex
	reqs []agent.DecisionRequest
}

func (a *captureAgent) Decide(ctx context.Context, req agent.DecisionRequest) (agent.Decision, error) {
	a.mu.Lock()
	a.reqs = append(a.reqs, req)
	act := req.Actions[0]
	a.mu.Unlock()
	return agent.Decision{Action: act, Raw: act.Name}, nil
}

func TestUnchangedScreenDropsLastAction(t *testing.T) {
	g := &fakeGame{}
	ag := &captureAgent{}
	r := New(g, ag, nil, Config{Goal: "go", Model: "test", MaxDecisions: 2, History: 4})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	r.Start(ctx)
	r.Wait()
	ag.mu.Lock()
	defer ag.mu.Unlock()
	if len(ag.reqs) != 2 {
		t.Fatalf("reqs %d", len(ag.reqs))
	}
	if got := names(ag.reqs[0].Actions); strings.Join(got, ",") != "A,WAIT" {
		t.Fatalf("first actions %v", got)
	}
	if got := names(ag.reqs[1].Actions); strings.Join(got, ",") != "WAIT" {
		t.Fatalf("second actions %v (last no-op A should be dropped)", got)
	}
	if !ag.reqs[1].UnchangedScreen {
		t.Fatal("expected unchanged screen on second step")
	}
}

func names(actions []game.Action) []string {
	out := make([]string, len(actions))
	for i, a := range actions {
		out[i] = a.Name
	}
	return out
}
