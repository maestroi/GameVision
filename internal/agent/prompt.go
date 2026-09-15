package agent

import (
	"fmt"
	"strings"

	"github.com/maestroi/gamevision/pkg/game"
)

// RenderPrompt builds a small visual-policy prompt. It must stay small: the
// screenshot carries state. Do not inject walkthroughs, maps, or RAM facts.
func RenderPrompt(req DecisionRequest) string {
	var b strings.Builder
	b.WriteString("You are controlling a video game by looking at the current screen.\n\n")
	if req.Game != "" {
		b.WriteString("Game:\n")
		b.WriteString(req.Game)
		b.WriteString("\n\n")
	}
	b.WriteString("Current goal:\n")
	if req.Goal != "" {
		b.WriteString(req.Goal)
	} else {
		b.WriteString("Progress as far as possible.")
	}
	b.WriteString("\n\nValid controller actions:\n\n")
	for _, a := range req.Actions {
		b.WriteString(a.Name)
		b.WriteByte('\n')
	}
	b.WriteString("\nRecent actions (already done — do not copy them):\n")
	if len(req.History) == 0 {
		b.WriteString("(none)\n")
	} else {
		for _, a := range req.History {
			b.WriteString(a.Name)
			b.WriteByte('\n')
		}
	}
	b.WriteString("\nChoose from the screenshot.\n")
	b.WriteString("Walking around a room, town, or path: use UP, DOWN, LEFT, or RIGHT. Do not mash A.\n")
	b.WriteString("A dialogue box with text and a blinking ▼ / triangle: press A once to advance, then look again.\n")
	b.WriteString("Title or start screen: START or A.\n")
	b.WriteString("WAIT is rare (animation with no prompt).\n")
	b.WriteString("Black bars on a Game Boy shot are off-camera, not a hallway.\n")
	b.WriteString("If you can see a door, stairs, or opening at the edge of the floor, walk toward it.\n")
	if name, n := trailingStreak(req.History); n >= 3 {
		fmt.Fprintf(&b, "You already pressed %s %d times in a row. Do not press %s again unless the screenshot still clearly requires it.\n", name, n, name)
	}
	if req.UnchangedScreen {
		b.WriteString("The picture did not change after your last action. Repeat is failing; pick a different action (usually a different d-pad direction, not more A).\n")
	}
	b.WriteString("\nReturn exactly one valid action as JSON {\"action\":\"NAME\"} and nothing else.\n")
	return b.String()
}

func trailingStreak(history []game.Action) (string, int) {
	if len(history) == 0 {
		return "", 0
	}
	name := strings.ToUpper(strings.TrimSpace(history[len(history)-1].Name))
	n := 0
	for i := len(history) - 1; i >= 0; i-- {
		if strings.ToUpper(strings.TrimSpace(history[i].Name)) != name {
			break
		}
		n++
	}
	return name, n
}

func HistoryNames(history []game.Action) []string {
	out := make([]string, len(history))
	for i, a := range history {
		out[i] = a.Name
	}
	return out
}
