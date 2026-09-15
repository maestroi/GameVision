package agent

import (
	"context"
	"strings"
	"time"

	"github.com/maestroi/gamevision/internal/inference"
	"github.com/maestroi/gamevision/pkg/game"
)

// DecisionRequest is one visual-policy query. The screenshot remains the
// source of truth; the extra fields are only short-lived memory inferred from
// earlier screenshots and action outcomes.
type DecisionRequest struct {
	Observation  game.Observation
	Game         string
	Goal         string
	Actions      []game.Action
	History      []game.Action
	Subgoal      string
	LastScene    string
	LastOutcome  string
	LastExpected string
}

// Decision is a parsed short-horizon controller plan plus enough telemetry to
// compare models. Repeat is a requested bounded burst; runtime may shorten it
// after visual verification.
type Decision struct {
	Action     game.Action
	Repeat     int
	Scene      string
	Subgoal    string
	Expected   string
	Confidence float64
	Raw        string
	Invalid    bool
	Retried    bool
	Inference  time.Duration
	Parse      time.Duration
	Error      string
	Timeout    bool
}

// Agent chooses a short-horizon action from pixels. It is still vision-first:
// persistent state is limited to model-inferred scene/subgoal context and
// observed consequences of previous inputs.
type Agent interface {
	Decide(ctx context.Context, req DecisionRequest) (Decision, error)
}

// VisionAgent asks an external VLM for a small visual control decision.
type VisionAgent struct {
	Client Completer
}

// Completer is satisfied by inference.Client.
type Completer interface {
	Complete(ctx context.Context, prompt string, png []byte) (inference.Result, error)
}

func New(client Completer) *VisionAgent {
	return &VisionAgent{Client: client}
}

func (a *VisionAgent) Decide(ctx context.Context, req DecisionRequest) (Decision, error) {
	prompt := RenderPrompt(req)
	res, err := a.Client.Complete(ctx, prompt, req.Observation.Image)
	if err != nil {
		d := Decision{
			Action:    fallback(req.Actions),
			Repeat:    1,
			Invalid:   true,
			Inference: res.Latency,
			Error:     err.Error(),
			Timeout:   isTimeout(err),
		}
		return d, nil
	}

	parseStart := time.Now()
	out, ok := ParsePolicy(res.Text, req.Actions)
	parseDur := time.Since(parseStart)
	if ok {
		return decisionFromPolicy(out, res.Text, res.Latency, parseDur, false), nil
	}

	retryPrompt := prompt + "\n\nYour previous reply was invalid. Return exactly one JSON object using a valid action and no prose."
	res2, err := a.Client.Complete(ctx, retryPrompt, req.Observation.Image)
	if err != nil {
		return Decision{
			Action:    fallback(req.Actions),
			Repeat:    1,
			Raw:       res.Text,
			Invalid:   true,
			Retried:   true,
			Inference: res.Latency + res2.Latency,
			Parse:     parseDur,
			Error:     err.Error(),
			Timeout:   isTimeout(err),
		}, nil
	}
	parseStart = time.Now()
	out, ok = ParsePolicy(res2.Text, req.Actions)
	parseDur += time.Since(parseStart)
	if !ok {
		return Decision{
			Action:    fallback(req.Actions),
			Repeat:    1,
			Raw:       res.Text + "\n--- retry ---\n" + res2.Text,
			Invalid:   true,
			Retried:   true,
			Inference: res.Latency + res2.Latency,
			Parse:     parseDur,
		}, nil
	}
	d := decisionFromPolicy(out, res2.Text, res.Latency+res2.Latency, parseDur, true)
	return d, nil
}

func decisionFromPolicy(out PolicyOutput, raw string, inferenceDur, parseDur time.Duration, retried bool) Decision {
	return Decision{
		Action:     out.Action,
		Repeat:     out.Repeat,
		Scene:      out.Scene,
		Subgoal:    out.Subgoal,
		Expected:   out.Expected,
		Confidence: out.Confidence,
		Raw:        raw,
		Retried:    retried,
		Inference:  inferenceDur,
		Parse:      parseDur,
	}
}

func fallback(actions []game.Action) game.Action {
	for _, a := range actions {
		if strings.EqualFold(a.Name, "WAIT") {
			return a
		}
	}
	return game.Action{Name: "WAIT"}
}

func isTimeout(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "timeout") || strings.Contains(s, "deadline exceeded")
}
