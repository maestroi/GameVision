package agent

import (
	"context"
	"strings"
	"time"

	"github.com/maestroi/gamevision/internal/inference"
	"github.com/maestroi/gamevision/pkg/game"
)

// DecisionRequest is one mostly-stateless visual policy query.
type DecisionRequest struct {
	Observation game.Observation
	Game        string
	Goal        string
	Actions     []game.Action
	History     []game.Action
	// UnchangedScreen is true when this screenshot matches the previous one
	// (last action hit a wall, was blocked, or did nothing).
	UnchangedScreen bool
}

// Decision is a parsed controller action plus enough telemetry to compare models.
type Decision struct {
	Action    game.Action
	Raw       string
	Invalid   bool
	Retried   bool
	Inference time.Duration
	Parse     time.Duration
	Error     string
	Timeout   bool
}

// Agent chooses one action from pixels. It is a visual policy, not a chatbot.
type Agent interface {
	Decide(ctx context.Context, req DecisionRequest) (Decision, error)
}

// VisionAgent asks an external VLM for a single action name.
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
			Invalid:   true,
			Inference: res.Latency,
			Error:     err.Error(),
			Timeout:   isTimeout(err),
		}
		return d, nil
	}

	parseStart := time.Now()
	action, ok := ParseAction(res.Text, req.Actions)
	parseDur := time.Since(parseStart)
	if ok {
		return Decision{
			Action:    action,
			Raw:       res.Text,
			Inference: res.Latency,
			Parse:     parseDur,
		}, nil
	}

	retryPrompt := prompt + "\n\nYour previous reply was invalid. Return exactly one JSON object like {\"action\":\"A\"} using a valid action, and nothing else."
	res2, err := a.Client.Complete(ctx, retryPrompt, req.Observation.Image)
	if err != nil {
		return Decision{
			Action:    fallback(req.Actions),
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
	action, ok = ParseAction(res2.Text, req.Actions)
	parseDur += time.Since(parseStart)
	if !ok {
		return Decision{
			Action:    fallback(req.Actions),
			Raw:       res.Text + "\n--- retry ---\n" + res2.Text,
			Invalid:   true,
			Retried:   true,
			Inference: res.Latency + res2.Latency,
			Parse:     parseDur,
		}, nil
	}
	return Decision{
		Action:    action,
		Raw:       res2.Text,
		Retried:   true,
		Inference: res.Latency + res2.Latency,
		Parse:     parseDur,
	}, nil
}

func fallback(actions []game.Action) game.Action {
	for _, a := range actions {
		if strings.EqualFold(a.Name, "WAIT") {
			return a
		}
	}
	if len(actions) == 0 {
		return game.Action{Name: "WAIT"}
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
