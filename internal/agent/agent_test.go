package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/maestroi/gamevision/internal/inference"
	"github.com/maestroi/gamevision/pkg/game"
)

func gbActions() []game.Action {
	names := []string{"UP", "DOWN", "LEFT", "RIGHT", "A", "B", "START", "SELECT", "WAIT"}
	out := make([]game.Action, len(names))
	for i, n := range names {
		out[i] = game.Action{Name: n}
	}
	return out
}

func TestParseActionJSON(t *testing.T) {
	a, ok := ParseAction(`{"action":"RIGHT"}`, gbActions())
	if !ok || a.Name != "RIGHT" {
		t.Fatalf("got %+v ok=%v", a, ok)
	}
}

func TestParseActionPlain(t *testing.T) {
	a, ok := ParseAction("  a  ", gbActions())
	if !ok || a.Name != "A" {
		t.Fatalf("got %+v ok=%v", a, ok)
	}
}

func TestParseActionUnambiguousToken(t *testing.T) {
	a, ok := ParseAction("press RIGHT now", gbActions())
	if !ok || a.Name != "RIGHT" {
		t.Fatalf("got %+v ok=%v", a, ok)
	}
}

func TestParseActionAmbiguousRejected(t *testing.T) {
	if _, ok := ParseAction("UP then DOWN", gbActions()); ok {
		t.Fatal("expected reject")
	}
}

func TestParseActionUnknownRejected(t *testing.T) {
	if _, ok := ParseAction("JUMP", gbActions()); ok {
		t.Fatal("expected reject")
	}
}

func TestParsePolicyRichJSON(t *testing.T) {
	p, ok := ParsePolicy(`{"scene":"Overworld","subgoal":"reach door","action":"RIGHT","repeat":3,"expected":"move closer","confidence":0.8}`, gbActions())
	if !ok {
		t.Fatal("expected rich policy to parse")
	}
	if p.Action.Name != "RIGHT" || p.Repeat != 3 || p.Scene != "overworld" || p.Subgoal != "reach door" || p.Expected != "move closer" || p.Confidence != 0.8 {
		t.Fatalf("%+v", p)
	}
}

func TestParsePolicyClampsRepeat(t *testing.T) {
	p, ok := ParsePolicy(`{"action":"LEFT","repeat":99}`, gbActions())
	if !ok || p.Repeat != 4 {
		t.Fatalf("%+v ok=%v", p, ok)
	}
}

type scriptedCompleter struct {
	replies []inference.Result
	errs    []error
	i       int
}

func (s *scriptedCompleter) Complete(ctx context.Context, prompt string, png []byte) (inference.Result, error) {
	if s.i >= len(s.replies) {
		return inference.Result{}, nil
	}
	res := s.replies[s.i]
	var err error
	if s.i < len(s.errs) {
		err = s.errs[s.i]
	}
	s.i++
	_ = prompt
	_ = png
	_ = ctx
	return res, err
}

func TestVisionAgentRetryThenWAIT(t *testing.T) {
	a := New(&scriptedCompleter{
		replies: []inference.Result{{Text: "nope"}, {Text: "still nope"}},
	})
	d, err := a.Decide(context.Background(), DecisionRequest{
		Observation: game.Observation{Image: []byte("x")},
		Actions:     gbActions(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !d.Invalid || !d.Retried || d.Action.Name != "WAIT" || d.Repeat != 1 {
		t.Fatalf("%+v", d)
	}
}

func TestVisionAgentRetryRecovers(t *testing.T) {
	a := New(&scriptedCompleter{
		replies: []inference.Result{{Text: "nope"}, {Text: `{"scene":"menu","subgoal":"close menu","action":"B","repeat":1,"expected":"menu closes","confidence":0.9}`}},
	})
	d, err := a.Decide(context.Background(), DecisionRequest{
		Observation: game.Observation{Image: []byte("x")},
		Actions:     gbActions(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if d.Invalid || !d.Retried || d.Action.Name != "B" || d.Scene != "menu" || d.Subgoal != "close menu" {
		t.Fatalf("%+v", d)
	}
}

func TestRenderPromptKeepsVisualContinuityWithoutAntiRepeat(t *testing.T) {
	p := RenderPrompt(DecisionRequest{
		Game:         "Pokémon Red",
		Goal:         "Leave the house.",
		Subgoal:      "Reach the stairs.",
		LastScene:    "overworld",
		LastOutcome:  "visual_change",
		LastExpected: "move closer to stairs",
		Actions:      gbActions(),
		History:      []game.Action{{Name: "RIGHT"}, {Name: "RIGHT"}, {Name: "RIGHT"}},
	})
	for _, s := range []string{"Pokémon Red", "Leave the house.", "Reach the stairs.", "visual_change", "RIGHT", "WAIT", `"subgoal"`, `"repeat"`, "repeating a direction is normal", "Repeated d-pad movement is GOOD", "Black bars"} {
		if !strings.Contains(p, s) {
			t.Fatalf("prompt missing %q:\n%s", s, p)
		}
	}
	for _, bad := range []string{"already done", "Do not press RIGHT again", "A 3 times in a row"} {
		if strings.Contains(p, bad) {
			t.Fatalf("anti-repeat prompt leaked %q:\n%s", bad, p)
		}
	}
	if strings.Contains(strings.ToLower(p), "pallet") {
		t.Fatal("prompt must not include walkthrough content")
	}
}
