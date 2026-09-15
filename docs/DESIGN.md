# GameVision v1 design

GameVision is a vision-first game-agent runtime. The first experiment is Pokémon Red through gomeboy: a local VLM sees pixels and returns one controller action. It is not a Pokémon automation stack with a camera bolted on.

## Inspection summary

### Reuse conceptually from PokéPilot

- Headless gomeboy session owned by a game adapter, not by the model layer.
- Watch UI as a spectator/control plane: framebuffer plus session telemetry. It is never the timing source.
- OpenAI-compatible HTTP client (`/chat/completions`), with thinking disabled when the server honours it.
- Session artifacts: JSON summary, JSONL event log, optional frame dumps.
- Human vs agent input provenance (`source=human` / `source=agent`).
- Save states as scenario entry points only.
- CLI goal string as a run parameter, not a walkthrough.

### Do not reuse from PokéPilot

- RAM/HRAM parsing, map graphs, collision, party, inventory, battle, dialogue, menus.
- Skills, pathfinding, scripted boot-to-overworld, recovery, stagnation watchdogs that read Pokémon state.
- Planner/objective transactions, capability models, farm/orchestration.
- Large conversational prompts, injected facts, or route knowledge.
- Any hidden rescue that would mask vision failure.

### What gomeboy already provides

`pkg/gomeboy` already has the integration GameVision needs. No emulator fork is required for v1:

| Need | API |
|---|---|
| Framebuffer | `Frame()` RGB 160×144, plus `PNG()` / `Image()` |
| Input | `Press` / `Release` |
| Advance | `StepFrame` / `StepFrames` |
| Lifecycle | `New`, `Headless`, `WithROM`, `WithModel`, `Close` |
| Save states | `SaveState` / `LoadState` (and checked variants) |
| Spectator | `Spectator` is read-only; GameVision wraps its own UI around the same capture-after-step idea |

Do not use `WithoutVideo()` for this experiment. Do not read emulator memory from GameVision.

### What belongs in GameVision

- Generic `Game` / `Agent` / runtime loop.
- OpenAI-compatible VLM client. GameVision does not embed llama.cpp; on a single-model `llama-server` it may restart that process because there is no `/models/load`.
- Nearest-neighbor frame scaling and PNG encoding.
- Strict action parsing with WAIT fallback.
- Session recording and latency metrics.
- Pokémon Red adapter: GB buttons, hold/settle timing, optional CGB colorization, save-state load.
- Observational web UI with pause/resume/stop and manual buttons.

## Architecture

```text
cmd/gamevision
        │
        ▼
internal/runtime          observe → decide → apply
        │
        ├── pkg/game              Observation, Action, Game
        ├── internal/agent        visual policy + parser
        ├── internal/inference    OpenAI-compatible VLM HTTP
        ├── internal/vision       nearest-neighbor scale + PNG
        ├── internal/recording    sessions/, jsonl, frames
        ├── internal/metrics      per-decision and summary
        ├── internal/ui           spectator/control plane
        └── games/pokemonred      gomeboy adapter
```

The core packages do not know map IDs, party state, or Pokémon RAM. The adapter knows Game Boy buttons and how to talk to gomeboy. The prompt may name the game and the human goal; it must not include walkthroughs.

## Loop

```text
observe framebuffer
  → nearest-neighbor scale (default 2x)
  → PNG
  → VLM (stateless, last 8 actions)
  → parse one valid action (else WAIT)
  → press/hold/release or WAIT
  → settle
  → repeat
```

Timing is fixed and configurable (`button_hold_frames=6`, `settle_frames=12`). No battle/dialogue/menu special cases in v1.

## Model contract

External server only (`base_url`, `model`, `api_key`, `timeout`, `temperature`, `max_tokens`). Default `temperature=0`, `max_tokens=16` (enough for `{"action":"SELECT"}`; 4 tokens is too tight for JSON).

Invalid output never becomes an arbitrary emulator command. One controlled retry, then WAIT.
