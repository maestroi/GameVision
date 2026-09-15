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

func TestParseActionDoesNotAcceptJSONWithInvalidAction(t *testing.T) {
	if _, ok := ParseAction(`{"action":"JUMP"}`, gbActions()); ok {
		t.Fatal("expected reject")
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
	if !d.Invalid || !d.Retried || d.Action.Name != "WAIT" {
		t.Fatalf("%+v", d)
	}
}

func TestVisionAgentRetryRecovers(t *testing.T) {
	a := New(&scriptedCompleter{
		replies: []inference.Result{{Text: "nope"}, {Text: `{"action":"B"}`}},
	})
	d, err := a.Decide(context.Background(), DecisionRequest{
		Observation: game.Observation{Image: []byte("x")},
		Actions:     gbActions(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if d.Invalid || !d.Retried || d.Action.Name != "B" {
		t.Fatalf("%+v", d)
	}
}

func TestRenderPromptStaysSmallAndListsActions(t *testing.T) {
	p := RenderPrompt(DecisionRequest{
		Game:    "Pokémon Red",
		Goal:    "Leave the house.",
		Actions: gbActions(),
		History: []game.Action{{Name: "RIGHT"}, {Name: "A"}},
	})
	for _, s := range []string{"Pokémon Red", "Leave the house.", "RIGHT", "WAIT", `{"action":"NAME"}`, "Do not mash A", "blinking", "door, stairs"} {
		if !strings.Contains(p, s) {
			t.Fatalf("prompt missing %q:\n%s", s, p)
		}
	}
	if !strings.Contains(p, "already done") {
		t.Fatal("prompt must tell the model not to copy recent WAIT")
	}
	if !strings.Contains(p, "off-camera") {
		t.Fatal("prompt must mention off-camera black bars")
	}
	if strings.Contains(p, "did not change") {
		t.Fatal("unchanged-screen hint must be off by default")
	}
	if strings.Contains(p, "in a row") {
		t.Fatal("streak hint must be off for short history")
	}
	stuck := RenderPrompt(DecisionRequest{UnchangedScreen: true, Actions: gbActions()})
	if !strings.Contains(stuck, "did not change") {
		t.Fatal("unchanged screen should ask for a different direction")
	}
	spam := RenderPrompt(DecisionRequest{
		Actions: gbActions(),
		History: []game.Action{{Name: "A"}, {Name: "A"}, {Name: "A"}, {Name: "A"}},
	})
	if !strings.Contains(spam, "A 4 times") {
		t.Fatalf("streak hint missing:\n%s", spam)
	}
	if strings.Contains(strings.ToLower(p), "pallet") || strings.Contains(p, "map") {
		t.Fatal("prompt must not include walkthrough/map content")
	}
}
