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
	b.WriteString("You control a video game only by looking at the current screenshot.\n\n")
	if req.Game != "" {
		b.WriteString("Game: ")
		b.WriteString(req.Game)
		b.WriteString("\n")
	}
	b.WriteString("Human goal: ")
	if req.Goal != "" {
		b.WriteString(req.Goal)
	} else {
		b.WriteString("Progress as far as possible.")
	}
	b.WriteString("\n")
	if req.Subgoal != "" {
		b.WriteString("Current visual subgoal: ")
		b.WriteString(req.Subgoal)
		b.WriteString("\n")
	}
	if req.LastScene != "" {
		b.WriteString("Previous scene guess: ")
		b.WriteString(req.LastScene)
		b.WriteString("\n")
	}
	if req.LastOutcome != "" {
		b.WriteString("Result of the previous controller input: ")
		b.WriteString(req.LastOutcome)
		if req.LastExpected != "" {
			b.WriteString("; expected: ")
			b.WriteString(req.LastExpected)
		}
		b.WriteString("\n")
	}

	b.WriteString("\nValid controller actions:\n")
	for _, a := range req.Actions {
		b.WriteString(a.Name)
		b.WriteByte('\n')
	}
	b.WriteString("\nRecent controller inputs (context only; repeating a direction is normal):\n")
	if len(req.History) == 0 {
		b.WriteString("(none)\n")
	} else {
		for _, a := range req.History {
			b.WriteString(a.Name)
			b.WriteByte(' ')
		}
		b.WriteByte('\n')
	}

	b.WriteString("\nChoose a short visual subgoal and the next controller input from the screenshot.\n")
	b.WriteString("Repeated d-pad movement is GOOD when a visible path is clear. Use repeat 1-4 for UP/DOWN/LEFT/RIGHT to keep moving toward the same visible target.\n")
	b.WriteString("Use repeat=1 for A, B, START, SELECT, or WAIT. Dialogue text should normally advance with A once, then look again.\n")
	b.WriteString("If the previous input caused no_visual_change, reconsider the obstacle or direction; do not blindly repeat it.\n")
	b.WriteString("For a title/start screen use START or A. WAIT is mainly for an animation with no prompt.\n")
	b.WriteString("Black bars on a Game Boy shot are off-camera, not a hallway. If you can see a door, stairs, opening, menu choice, or other obvious target, make the subgoal about reaching/using it.\n")
	b.WriteString("Do not invent walkthrough facts or locations that are not visible or already established by the visual history.\n")
	b.WriteString("\nReturn exactly one compact JSON object and no prose:\n")
	b.WriteString(`{"scene":"overworld|dialogue|menu|battle|title|transition|unknown","subgoal":"short visible objective","action":"NAME","repeat":1,"expected":"visible result after the input","confidence":0.0}`)
	b.WriteString("\n")
	fmt.Fprint(&b, "For clear straight walking, repeat may be 2-4; otherwise keep repeat=1.\n")
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
