package agent

import (
	"fmt"
	"strings"

	"github.com/maestroi/gamevision/pkg/game"
)

// RenderPrompt builds a compact visual-control prompt. The screenshot is the
// source of truth; remembered scene/subgoal text is only continuity from prior
// visual decisions, never hidden game state.
func RenderPrompt(req DecisionRequest) string {
	var b strings.Builder
	b.WriteString("You are a visual-only video-game controller. Decide from the current screenshot plus the short visual history below.\n")
	if req.Game != "" {
		b.WriteString("Game: ")
		b.WriteString(req.Game)
		b.WriteByte('\n')
	}
	b.WriteString("Goal: ")
	if req.Goal != "" {
		b.WriteString(req.Goal)
	} else {
		b.WriteString("Progress as far as possible.")
	}
	b.WriteByte('\n')

	if req.LastScene != "" || req.Subgoal != "" {
		b.WriteString("Visual memory:")
		if req.LastScene != "" {
			b.WriteString(" scene=")
			b.WriteString(req.LastScene)
		}
		if req.Subgoal != "" {
			b.WriteString(" subgoal=")
			b.WriteString(req.Subgoal)
		}
		b.WriteByte('\n')
	}

	lastAction := ""
	if len(req.History) > 0 {
		lastAction = req.History[len(req.History)-1].Name
	}
	if req.LastOutcome != "" {
		b.WriteString("Previous result:")
		if lastAction != "" {
			b.WriteByte(' ')
			b.WriteString(lastAction)
		}
		b.WriteString(" -> ")
		b.WriteString(req.LastOutcome)
		if req.LastExpected != "" {
			b.WriteString(" (expected ")
			b.WriteString(req.LastExpected)
			b.WriteByte(')')
		}
		b.WriteByte('\n')
	}

	b.WriteString("Actions: ")
	for i, a := range req.Actions {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(a.Name)
	}
	b.WriteByte('\n')

	b.WriteString("Recent: ")
	if len(req.History) == 0 {
		b.WriteString("none")
	} else {
		for i, a := range req.History {
			if i > 0 {
				b.WriteByte(' ')
			}
			b.WriteString(a.Name)
		}
	}
	b.WriteByte('\n')

	b.WriteString("Rules:\n")
	b.WriteString("- Use only what is visible or established by the visual history. Do not use walkthrough knowledge.\n")
	b.WriteString("- Dialogue/text box with no visible choice: press A once, then look again. A menu or choice list is not plain dialogue.\n")
	b.WriteString("- Title/start prompt: use START or A. Menu/battle: follow the visible cursor/text.\n")
	b.WriteString("- Overworld: move toward a visible door, stairs, opening, path, NPC, or other useful target. Straight clear walking may repeat 2-4.\n")
	b.WriteString("- A/B/START/SELECT/WAIT always use repeat 1. Black bars are off-camera, not paths.\n")
	if req.LastOutcome == "no_visual_change" && lastAction != "" {
		fmt.Fprintf(&b, "- IMPORTANT: %s just caused no visual change. Treat it as blocked/unhelpful on this frame; choose another action unless WAIT is needed for a visible animation.\n", lastAction)
	} else {
		b.WriteString("- If an input causes no visual change, reconsider instead of blindly repeating it.\n")
	}
	b.WriteString("- If the scene visibly changed, replace any stale subgoal with a new visible one.\n")

	b.WriteString("Return exactly one compact JSON object, no prose:\n")
	b.WriteString(`{"s":"o|d|m|b|t|x|u","g":"2-5 word visible goal","a":"ACTION","r":1,"c":0.0}`)
	b.WriteByte('\n')
	b.WriteString("s codes: o=overworld d=dialogue m=menu b=battle t=title x=transition u=unknown. c is confidence 0..1.\n")
	return b.String()
}

// HistoryNames is useful in tests and telemetry adapters.
func HistoryNames(history []game.Action) []string {
	out := make([]string, len(history))
	for i, a := range history {
		out[i] = a.Name
	}
	return out
}
