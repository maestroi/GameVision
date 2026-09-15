# GameVision

A vision-driven local game agent. The first experiment is **Pokémon Red** through [gomeboy](https://github.com/maestroi/gomeboy): a local vision-language model sees rendered frames and issues Game Boy buttons.

```text
game frame → local VLM → controller action → visual verification → game → next frame
```

The point of v1 is not the strongest Pokémon player. It is to measure how far a fast local VLM can get when its primary observation is pixels.

PokéPilot is the opposite experiment (structured emulator memory). GameVision does not read Pokémon RAM, maps, party, inventory, or battle state.

## Loop

```bash
gamevision \
  --game pokemon-red \
  --rom /path/to/pokemon-red.gb \
  --base-url http://localhost:8002/v1 \
  --model Qwen3-VL-4B-Instruct \
  --goal "Start Pokémon Red and progress as far as possible." \
  --vision-scale 2
```

Then open `http://localhost:8099`. The page is observational: it does not clock the emulator.

Pause / resume / stop the agent from the UI. Manual buttons are logged as `source=human`; model actions as `source=agent`.

## Model server

GameVision does not embed llama.cpp. Point `--base-url` at an OpenAI-compatible multimodal endpoint.

The homelab deployment uses the RTX 4090 endpoint at `http://192.168.50.81:8002/v1` and expects `Qwen3-VL-4B-Instruct`. That physical port may also be used for a text-only model at other times, so the container does not trust the configured model name: it sends a real image request before starting and keeps probing while GameVision runs.

Useful models for local experiments:

- `Qwen3-VL-2B-Instruct`
- `Qwen3-VL-4B-Instruct`

Recommended sampling for the richer visual-policy reply: `--temperature 0 --max-tokens 96`.

## Switch models

If the server is llama.cpp **router** mode, `switch` unloads VRAM then `POST /models/load`.

For a single-model `llama-server`, model switching is host-specific and may restart that inference process. The debug UI exposes the same switch operation when the configured host supports it.

```bash
gamevision models
gamevision models switch Qwen3-VL-4B-Instruct
make models-restore   # restore the host's text model when desired
```

`gamevision bench` can compare 2B and 4B (`--swap-models=false` skips host-side switching).

## Homelab / Docker Swarm

`main` publishes `ghcr.io/maestroi/gamevision:latest` plus an immutable commit-SHA tag. The Swarm stack publishes only a private host-mode port; your existing homelab router can route `gamevision.labstack.cc` to that port exactly like the PokePilot deployment.

Defaults:

```text
image       ghcr.io/maestroi/gamevision:latest
host port   18082
container   8099
VLM         http://192.168.50.81:8002/v1
model       Qwen3-VL-4B-Instruct
ROM         /opt/gamevision/roms/pokemon_red.gb
state       /opt/gamevision
```

Deploy from a Swarm manager:

```bash
sudo mkdir -p /opt/gamevision/roms /opt/gamevision/sessions
sudo cp /path/to/pokemon_red.gb /opt/gamevision/roms/pokemon_red.gb

docker pull ghcr.io/maestroi/gamevision:latest
docker stack deploy --with-registry-auth -c deploy/swarm.yml gamevision
```

The container performs a real multimodal probe before binding the UI. If `:8002` is currently text-only/no-mmproj, it waits. If vision disappears later, the container stops GameVision and Swarm restarts into that same preflight gate. See [`deploy/README.md`](deploy/README.md) for the full homelab workflow and overrides.

## Timing

Defaults, all configurable:

| Flag | Default |
|---|---|
| `--button-hold-frames` | 16 (one Gen 1 walk tile) |
| `--settle-frames` | 12 |
| `--vision-scale` | 2 (320×288, nearest-neighbor) |
| `--history` | 8 |

There is no RAM-derived battle/dialogue/menu timing logic.

## Sessions

Each run writes:

```text
sessions/<timestamp>-pokemon-red-<model>/
  session.json
  decisions.jsonl
  frames/000001.png
```

`--save-decision-frames=true` by default. `--save-all-frames` is off.

The decision log includes vision-derived scene/subgoal context, requested/applied movement repeat, expected result, confidence, visual outcome, and perceptual change score.

## Scenarios

Reproducible starts use gomeboy save states. The state is not shown to the model.

```bash
gamevision \
  --rom /path/to/pokemon-red.gb \
  --scenario scenarios/pokemon-red/title-screen.json \
  --base-url http://localhost:8002/v1 \
  --model Qwen3-VL-4B-Instruct
```

Capture extra states from the web UI (**Save state**) while paused, then point a scenario JSON at the file. See `scenarios/pokemon-red/`.

## 2B vs 4B bench

```bash
gamevision bench \
  --rom /path/to/pokemon-red.gb \
  --scenario scenarios/pokemon-red/title-screen.json \
  --base-url http://localhost:8002/v1 \
  --models Qwen3-VL-2B-Instruct,Qwen3-VL-4B-Instruct \
  --max-tokens 96
```

Identical goal, prompt, scale, timing, temperature, token cap, and decision limit. Writes `comparison.json` with median/p95 latency, invalid outputs, and timeouts.

## Layout

```text
cmd/gamevision          CLI
pkg/game                Observation / Action / Game
internal/agent          visual policy + strict parser
internal/inference      OpenAI-compatible VLM HTTP
internal/runtime        observe → decide → apply → visually verify
internal/vision         scaling, PNG, perceptual frame change
internal/recording      session artifacts
internal/metrics        latency summaries
internal/ui             spectator / control plane
games/pokemonred        gomeboy adapter
deploy                  container + Swarm deployment
```

gomeboy already exposes framebuffer, `Press`/`Release`, `StepFrame(s)`, and save states. GameVision consumes that API; it does not poke emulator internals.

## Build

Requires Go 1.26+. Local development expects a sibling checkout of gomeboy (`../gomeboy` via `go.mod` replace). The container build deliberately drops that local replace and downloads the pinned `github.com/maestroi/gomeboy v1.1.0` module, so the image build is self-contained.

```bash
make test
make run
make models-4b
make models-restore
make bench
```

`make help` lists the rest. Override the ROM with `ROM=/path/to/pokemon-red.gb` or `POKEMON_RED_ROM`.
