# GameVision homelab deployment

GameVision is deployed like the private PokePilot service: Docker Swarm owns the
container and publishes one host-mode port; the existing homelab router owns the
`gamevision.labstack.cc` hostname. This repository intentionally does not create
Traefik labels, DNS records, certificates, or a public route.

## Defaults

- image: `ghcr.io/maestroi/gamevision:latest`
- GameVision UI inside the container: `:8099`
- Swarm host port: `18082`
- vision endpoint: `http://192.168.50.81:8002/v1`
- model: `Qwen3-VL-4B-Instruct`
- ROM on the Swarm node: `/opt/gamevision/roms/pokemon_red.gb`
- persistent data: `/opt/gamevision` -> `/data`

Point the existing internal router for `gamevision.labstack.cc` at the Swarm
node's HTTP port `18082`, the same way `pokemon.labstack.cc` is routed today.

## Vision availability gate

The container does not assume that `:8002` is a vision server just because of a
configured model name. Before starting the game it sends a real OpenAI-compatible
chat completion containing a tiny PNG.

If `192.168.50.81:8002` is currently running the text-only/no-mmproj model, the
probe fails and the task remains in preflight without binding `:8099`. When the
4B vision model becomes available, the task starts automatically.

After startup PID 1 repeats that multimodal probe every 30 seconds. If the vision
endpoint disappears or is switched back to a text-only model, it terminates
GameVision. Swarm restarts the task, which waits in preflight until multimodal
inference is available again.

Set `GAMEVISION_REQUIRE_VISION=0` and `GAMEVISION_WATCH_VISION=0` only for manual
debugging where this guard is unwanted.

## Prepare the Swarm node

```bash
sudo mkdir -p /opt/gamevision/roms /opt/gamevision/sessions
sudo cp /path/to/pokemon_red.gb /opt/gamevision/roms/pokemon_red.gb
```

The ROM is never included in the image or Git repository.

## Deploy

From the Swarm manager, with this repository checked out:

```bash
docker pull ghcr.io/maestroi/gamevision:latest
docker stack deploy --with-registry-auth -c deploy/swarm.yml gamevision
```

Useful checks:

```bash
docker stack services gamevision
docker service logs -f gamevision_app
curl http://127.0.0.1:18082/status
```

While the non-vision model is loaded on `:8002`, logs should show the vision
preflight retrying and the `curl` will not connect. Once Qwen3-VL-4B is serving
multimodal requests, the UI becomes available on port `18082` and therefore on
`gamevision.labstack.cc` through the existing router.

## Configuration

Override stack defaults from the manager environment before `docker stack deploy`:

```bash
export GAMEVISION_IMAGE=ghcr.io/maestroi/gamevision:latest
export GAMEVISION_BASE_URL=http://192.168.50.81:8002/v1
export GAMEVISION_MODEL=Qwen3-VL-4B-Instruct
export GAMEVISION_PORT=18082
export GAMEVISION_ROM=/opt/gamevision/roms/pokemon_red.gb
export GAMEVISION_STATE_DIR=/opt/gamevision
export GAMEVISION_GOAL='Start Pokémon Red and progress as far as possible.'
```

Optional inference settings include `GAMEVISION_API_KEY`,
`GAMEVISION_MAX_TOKENS`, `GAMEVISION_TIMEOUT`, `GAMEVISION_TEMPERATURE`, and the
`GAMEVISION_VISION_*` probe/watch intervals in `swarm.yml`.

## Updating

`main` publishes both `:latest` and an immutable commit-SHA tag to GHCR. To roll
the homelab service forward:

```bash
docker pull ghcr.io/maestroi/gamevision:latest
docker stack deploy --with-registry-auth -c deploy/swarm.yml gamevision
```

For a pinned deployment use, for example:

```bash
GAMEVISION_IMAGE=ghcr.io/maestroi/gamevision:<git-sha> \
  docker stack deploy --with-registry-auth -c deploy/swarm.yml gamevision
```
