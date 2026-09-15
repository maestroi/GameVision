# GameVision

A vision-driven local game agent. The first experiment is **Pokémon Red** through [gomeboy](https://github.com/maestroi/gomeboy): a local vision-language model sees rendered frames and issues Game Boy buttons.

```text
game frame → local VLM → controller action → game → next frame
```

The point of v1 is not the strongest Pokémon player. It is to measure how far a fast local VLM can get when its primary observation is pixels.

PokéPilot is the opposite experiment (structured emulator memory). GameVision does not read Pokémon RAM, maps, party, inventory, or battle state.

## Loop

```bash
gamevision \
  --game pokemon-red \
  --rom /path/to/pokemon-red.gb \
  --base-url http://localhost:8002/v1 \
  --model Qwen3-VL-2B-Instruct \
  --goal "Start Pokémon Red and progress as far as possible." \
  --vision-scale 2
```

Then open `http://localhost:8099`. The page is observational: it does not clock the emulator.

Pause / resume / stop the agent from the UI. Manual buttons are logged as `source=human`; model actions as `source=agent`.

## Model server

GameVision does not embed llama.cpp. Point `--base-url` at an OpenAI-compatible multimodal endpoint.

On this machine the default is `http://localhost:8002/v1`. That port is **single-model** `llama-server` from LM Studio, currently `qwen3.8-27b` with `--no-mmproj` (text only, `vision: false`). `GET /models` lists only the loaded alias. `POST /models/load` is 404 — there is no router catalog.

Start with:

- `Qwen3-VL-2B-Instruct`
- `Qwen3-VL-4B-Instruct`

Recommended sampling: `--temperature 0 --max-tokens 16`.

## Switch models

If the server is llama.cpp **router** mode, `switch` unloads VRAM then `POST /models/load`.

If it is this host's single-model server, `switch` downloads the Qwen VL GGUF with curl (this LM Studio `llama-server` has no HTTPS, so `-hf` cannot work), **then** stops `qwen38-solo.service` and starts a transient `gamevision-llm` with `--model` and `--mmproj`. That is disruptive to anything else using :8002.

```bash
gamevision models
gamevision models switch Qwen3-VL-4B-Instruct
make models-restore   # qwen3.8-27b back on :8002
```

There are no Qwen3-VL GGUFs in the LM Studio cache by default; the first VL switch downloads `Qwen3VL-*-Instruct-Q4_K_M.gguf` plus `mmproj-*-F16.gguf` (~3.3GB for 4B). You will see curl progress on stderr. Restore the coding model before leaving the session.

The debug UI has the same control. `gamevision bench` switches between 2B and 4B (`--swap-models=false` skips it).


## Timing

Defaults, all configurable:

| Flag | Default |
|---|---|
| `--button-hold-frames` | 16 (one Gen 1 walk tile) |
| `--settle-frames` | 12 |
| `--vision-scale` | 2 (320×288, nearest-neighbor) |
| `--history` | 8 |

There is no battle/dialogue/menu special-case timing in v1.

## Sessions

Each run writes:

```text
sessions/<timestamp>-pokemon-red-<model>/
  session.json
  decisions.jsonl
  frames/000001.png
```

`--save-decision-frames=true` by default. `--save-all-frames` is off.

## Scenarios

Reproducible starts use gomeboy save states. The state is not shown to the model.

```bash
gamevision \
  --rom /path/to/pokemon-red.gb \
  --scenario scenarios/pokemon-red/title-screen.json \
  --base-url http://localhost:8002/v1 \
  --model Qwen3-VL-2B-Instruct
```

Capture extra states from the web UI (**Save state**) while paused, then point a scenario JSON at the file. See `scenarios/pokemon-red/`.

## 2B vs 4B bench

```bash
gamevision bench \
  --rom /path/to/pokemon-red.gb \
  --scenario scenarios/pokemon-red/title-screen.json \
  --base-url http://localhost:8002/v1 \
  --models Qwen3-VL-2B-Instruct,Qwen3-VL-4B-Instruct
```

Identical goal, prompt, scale, timing, temperature, token cap, and decision limit. Writes `comparison.json` with median/p95 latency, invalid outputs, and timeouts.

## Layout

```text
cmd/gamevision          CLI
pkg/game                Observation / Action / Game
internal/agent          visual policy + strict parser
internal/inference      OpenAI-compatible VLM client
internal/runtime        observe → decide → apply
internal/vision         nearest-neighbor scale + PNG
internal/recording      session artifacts
internal/metrics        latency summaries
internal/ui             spectator / control plane
games/pokemonred        gomeboy adapter
```

gomeboy already exposes framebuffer, `Press`/`Release`, `StepFrame(s)`, and save states. GameVision consumes that API; it does not poke emulator internals.

## Build

Requires Go 1.26+. This repo expects a sibling checkout of gomeboy (`../gomeboy` via `go.mod` replace).

```bash
make test
make run
make models-4b       # restart :8002 with Qwen3-VL-4B (stops qwen38-solo)
make models-restore  # put qwen3.8-27b back
make bench
```

`make help` lists the rest. Override the ROM with `ROM=/path/to/pokemon-red.gb` or `POKEMON_RED_ROM`.
