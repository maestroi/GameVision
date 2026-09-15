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
	Scene           string  `json:"scene"`
	SceneShort      string  `json:"s"`
	Subgoal         string  `json:"subgoal"`
	SubgoalShort    string  `json:"g"`
	Action          string  `json:"action"`
	ActionShort     string  `json:"a"`
	Repeat          int     `json:"repeat"`
	RepeatShort     int     `json:"r"`
	Expected        string  `json:"expected"`
	Confidence      float64 `json:"confidence"`
	ConfidenceShort float64 `json:"c"`
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

// ParsePolicy accepts the compact controller contract as well as the older
// verbose JSON contract and plain-action replies. Compact replies keep model
// decode time low while preserving the visual scene/subgoal memory used by the
// runtime.
func ParsePolicy(raw string, actions []game.Action) (PolicyOutput, bool) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return PolicyOutput{}, false
	}

	if obj, ok := jsonObject(text); ok {
		var p policyJSON
		if err := json.Unmarshal([]byte(obj), &p); err == nil {
			actionName := firstNonEmpty(p.Action, p.ActionShort)
			if action, ok := allowedAction(actionName, actions); ok {
				repeat := p.Repeat
				if repeat == 0 {
					repeat = p.RepeatShort
				}
				if repeat <= 0 {
					repeat = 1
				}
				if repeat > 4 {
					repeat = 4
				}

				confidence := p.Confidence
				if confidence == 0 && p.ConfidenceShort != 0 {
					confidence = p.ConfidenceShort
				}
				if confidence > 1 && confidence <= 100 {
					confidence /= 100
				}
				if confidence < 0 {
					confidence = 0
				}
				if confidence > 1 {
					confidence = 1
				}

				scene := normalizeScene(firstNonEmpty(p.Scene, p.SceneShort))
				subgoal := clip(strings.TrimSpace(firstNonEmpty(p.Subgoal, p.SubgoalShort)), 160)
				expected := clip(strings.TrimSpace(p.Expected), 200)
				if expected == "" {
					expected = defaultExpected(scene, action.Name)
				}
				return PolicyOutput{
					Scene:      scene,
					Subgoal:    subgoal,
					Action:     action,
					Repeat:     repeat,
					Expected:   expected,
					Confidence: confidence,
				}, true
			}
		}
	}

	action, ok := ParseAction(text, actions)
	if !ok {
		return PolicyOutput{}, false
	}
	return PolicyOutput{Action: action, Repeat: 1, Expected: defaultExpected("", action.Name)}, true
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func normalizeScene(scene string) string {
	s := strings.ToLower(strings.TrimSpace(scene))
	switch s {
	case "o", "overworld":
		return "overworld"
	case "d", "dialog", "dialogue", "text":
		return "dialogue"
	case "m", "menu":
		return "menu"
	case "b", "battle", "combat":
		return "battle"
	case "t", "title", "start":
		return "title"
	case "x", "transition", "loading", "animation":
		return "transition"
	case "u", "unknown", "":
		return "unknown"
	default:
		return clip(s, 40)
	}
}

func defaultExpected(scene, action string) string {
	a := strings.ToUpper(strings.TrimSpace(action))
	switch scene {
	case "dialogue":
		if a == "A" || a == "B" {
			return "dialogue advances or closes"
		}
	case "title":
		if a == "START" || a == "A" {
			return "screen advances"
		}
	case "menu", "battle":
		switch a {
		case "UP", "DOWN", "LEFT", "RIGHT":
			return "visible selection moves"
		case "A":
			return "visible selection activates"
		case "B":
			return "menu or prompt backs out"
		}
	}
	switch a {
	case "UP", "DOWN", "LEFT", "RIGHT":
		return "player or view moves"
	case "A":
		return "visible interaction advances"
	case "B":
		return "visible prompt or menu backs out"
	case "START", "SELECT":
		return "screen or menu changes"
	case "WAIT":
		return "visible animation advances"
	default:
		return "visible state changes"
	}
}

func clip(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max]
}
