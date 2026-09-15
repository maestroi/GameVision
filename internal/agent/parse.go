package agent

import (
	"encoding/json"
	"regexp"
	"strings"
	"unicode"

	"github.com/maestroi/gamevision/pkg/game"
)

var tokenRe = regexp.MustCompile(`[A-Za-z]+`)

type actionJSON struct {
	Action string `json:"action"`
}

type policyJSON struct {
	Scene      string  `json:"scene"`
	Subgoal    string  `json:"subgoal"`
	Action     string  `json:"action"`
	Repeat     int     `json:"repeat"`
	Expected   string  `json:"expected"`
	Confidence float64 `json:"confidence"`
}

// PolicyOutput is the model's short-horizon visual control decision. All fields
// are inferred from pixels and the human goal; none are emulator state.
type PolicyOutput struct {
	Scene      string
	Subgoal    string
	Action     game.Action
	Repeat     int
	Expected   string
	Confidence float64
}

// ParsePolicy parses the richer visual-policy contract while remaining
// backward compatible with the original {"action":"..."} and plain-action
// replies. Repeat is clamped to a small bounded burst; runtime applies stricter
// controller-specific limits before execution.
func ParsePolicy(raw string, actions []game.Action) (PolicyOutput, bool) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return PolicyOutput{}, false
	}

	if obj, ok := jsonObject(text); ok {
		var p policyJSON
		if err := json.Unmarshal([]byte(obj), &p); err == nil {
			if action, ok := allowedAction(p.Action, actions); ok {
				repeat := p.Repeat
				if repeat <= 0 {
					repeat = 1
				}
				if repeat > 4 {
					repeat = 4
				}
				confidence := p.Confidence
				if confidence < 0 {
					confidence = 0
				}
				if confidence > 1 {
					confidence = 1
				}
				return PolicyOutput{
					Scene:      clip(strings.ToLower(strings.TrimSpace(p.Scene)), 40),
					Subgoal:    clip(strings.TrimSpace(p.Subgoal), 160),
					Action:     action,
					Repeat:     repeat,
					Expected:   clip(strings.TrimSpace(p.Expected), 200),
					Confidence: confidence,
				}, true
			}
		}
	}

	action, ok := ParseAction(text, actions)
	if !ok {
		return PolicyOutput{}, false
	}
	return PolicyOutput{Action: action, Repeat: 1}, true
}

// ParseAction maps model text onto the provided action space. Anything outside
// that space is invalid. The parser never invents emulator commands.
func ParseAction(raw string, actions []game.Action) (game.Action, bool) {
	allowed := make(map[string]game.Action, len(actions))
	for _, a := range actions {
		allowed[strings.ToUpper(strings.TrimSpace(a.Name))] = a
	}
	if len(allowed) == 0 {
		return game.Action{}, false
	}

	text := strings.TrimSpace(raw)
	if text == "" {
		return game.Action{}, false
	}

	if name, ok := jsonAction(text); ok {
		if a, ok := allowed[name]; ok {
			return a, true
		}
		return game.Action{}, false
	}

	upper := strings.ToUpper(text)
	if a, ok := allowed[upper]; ok {
		return a, true
	}

	var found []string
	seen := map[string]bool{}
	for _, tok := range tokenRe.FindAllString(upper, -1) {
		if _, ok := allowed[tok]; !ok {
			continue
		}
		if !seen[tok] {
			seen[tok] = true
			found = append(found, tok)
		}
	}
	if len(found) == 1 {
		return allowed[found[0]], true
	}
	return game.Action{}, false
}

func allowedAction(name string, actions []game.Action) (game.Action, bool) {
	want := strings.ToUpper(strings.TrimSpace(name))
	if want == "" {
		return game.Action{}, false
	}
	for _, r := range want {
		if !unicode.IsLetter(r) {
			return game.Action{}, false
		}
	}
	for _, a := range actions {
		if strings.EqualFold(strings.TrimSpace(a.Name), want) {
			return a, true
		}
	}
	return game.Action{}, false
}

func jsonAction(text string) (string, bool) {
	obj, ok := jsonObject(text)
	if !ok {
		return "", false
	}
	var parsed actionJSON
	if err := json.Unmarshal([]byte(obj), &parsed); err != nil {
		return "", false
	}
	name := strings.ToUpper(strings.TrimSpace(parsed.Action))
	if name == "" {
		return "", false
	}
	for _, r := range name {
		if !unicode.IsLetter(r) {
			return "", false
		}
	}
	return name, true
}

func jsonObject(text string) (string, bool) {
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start < 0 || end <= start {
		return "", false
	}
	return text[start : end+1], true
}

func clip(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max]
}
