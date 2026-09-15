package runtime

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/maestroi/gamevision/internal/agent"
	"github.com/maestroi/gamevision/pkg/game"
)

type fakeGame struct {
	mu         sync.Mutex
	frame      uint64
	position   int
	blockAfter int
	applied    []string
}

func (f *fakeGame) Name() string { return "Fake" }
func (f *fakeGame) Actions() []game.Action {
	return []game.Action{{Name: "RIGHT"}, {Name: "A"}, {Name: "WAIT"}}
}
func (f *fakeGame) Observe(ctx context.Context) (game.Observation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.frame++
	return game.Observation{Image: positionPNG(f.position), Width: 32, Height: 24, Frame: f.frame}, nil
}
func (f *fakeGame) Apply(ctx context.Context, action game.Action) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.applied = append(f.applied, action.Name)
	if action.Name == "RIGHT" && (f.blockAfter <= 0 || f.position < f.blockAfter) {
		f.position++
	}
	return nil
}
func (f *fakeGame) Close() error { return nil }
func (f *fakeGame) PreviewPNG() ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return positionPNG(f.position), nil
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
	return agent.Decision{Action: game.Action{Name: name}, Repeat: 1, Raw: name}, nil
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
	a.mu.Unlock()
	return agent.Decision{
		Action:   game.Action{Name: "RIGHT"},
		Repeat:   1,
		Scene:    "overworld",
		Subgoal:  "move right",
		Expected: "move right",
	}, nil
}

func TestRuntimeDoesNotSuppressRepeatedDirection(t *testing.T) {
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
	for i, req := range ag.reqs {
		if got := strings.Join(names(req.Actions), ","); got != "RIGHT,A,WAIT" {
			t.Fatalf("request %d actions %s", i, got)
		}
	}
	if ag.reqs[1].Subgoal != "move right" || ag.reqs[1].LastScene != "overworld" || ag.reqs[1].LastOutcome != "visual_change" {
		t.Fatalf("second request lost visual continuity: %+v", ag.reqs[1])
	}
}

type burstAgent struct {
	action string
	repeat int
}

func (a *burstAgent) Decide(ctx context.Context, req agent.DecisionRequest) (agent.Decision, error) {
	return agent.Decision{
		Action:   game.Action{Name: a.action},
		Repeat:   a.repeat,
		Scene:    "overworld",
		Subgoal:  "walk toward exit",
		Expected: "keep moving",
	}, nil
}

func TestVerifiedBurstStopsWhenMovementIsBlocked(t *testing.T) {
	g := &fakeGame{blockAfter: 2}
	r := New(g, &burstAgent{action: "RIGHT", repeat: 4}, nil, Config{Goal: "go", Model: "test", MaxDecisions: 1, History: 8})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	r.Start(ctx)
	r.Wait()
	snap := r.Snapshot()
	if snap.LastRepeat != 3 {
		t.Fatalf("applied repeat=%d want 3 (two moves plus blocked probe)", snap.LastRepeat)
	}
	if snap.LastOutcome != "no_visual_change" {
		t.Fatalf("outcome=%q", snap.LastOutcome)
	}
	g.mu.Lock()
	applied := append([]string(nil), g.applied...)
	g.mu.Unlock()
	if got := strings.Join(applied, ","); got != "RIGHT,RIGHT,RIGHT" {
		t.Fatalf("applied=%s", got)
	}
}

func TestNonDirectionalBurstIsForcedToOne(t *testing.T) {
	g := &fakeGame{}
	r := New(g, &burstAgent{action: "A", repeat: 4}, nil, Config{Goal: "go", Model: "test", MaxDecisions: 1, History: 8})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	r.Start(ctx)
	r.Wait()
	if got := r.Snapshot().LastRepeat; got != 1 {
		t.Fatalf("repeat=%d", got)
	}
	g.mu.Lock()
	n := len(g.applied)
	g.mu.Unlock()
	if n != 1 {
		t.Fatalf("applied=%d", n)
	}
}

func names(actions []game.Action) []string {
	out := make([]string, len(actions))
	for i, a := range actions {
		out[i] = a.Name
	}
	return out
}

func positionPNG(position int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, 32, 24))
	for y := 0; y < 24; y++ {
		for x := 0; x < 32; x++ {
			img.Set(x, y, color.RGBA{R: 16, G: 16, B: 16, A: 255})
		}
	}
	x0 := 2 + (position%5)*5
	for y := 8; y < 14; y++ {
		for x := x0; x < x0+5; x++ {
			img.Set(x, y, color.White)
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		panic(err)
	}
	return b.Bytes()
}
