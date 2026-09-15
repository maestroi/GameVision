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

func jsonAction(text string) (string, bool) {
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start < 0 || end <= start {
		return "", false
	}
	var obj actionJSON
	if err := json.Unmarshal([]byte(text[start:end+1]), &obj); err != nil {
		return "", false
	}
	name := strings.ToUpper(strings.TrimSpace(obj.Action))
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
