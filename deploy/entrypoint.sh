#!/bin/sh
set -eu

base_url="${GAMEVISION_BASE_URL:-http://192.168.50.81:8002/v1}"
model="${GAMEVISION_MODEL:-Qwen3-VL-4B-Instruct}"
api_key="${GAMEVISION_API_KEY:-}"
probe_timeout="${GAMEVISION_VISION_PROBE_TIMEOUT:-30}"
wait_interval="${GAMEVISION_VISION_WAIT_INTERVAL:-10}"
watch_interval="${GAMEVISION_VISION_WATCH_INTERVAL:-30}"
# 32x32 PNG. Large enough for common VLM image preprocessors while still tiny.
# A text-only llama-server started without mmproj rejects this multimodal request.
probe_png='iVBORw0KGgoAAAANSUhEUgAAACAAAAAgCAIAAAD8GO2jAAAAKElEQVR42u3NQQEAAAQEMPTvfErw2wqsk9SnqWcCgUAgEAgEAoHgygLH8QM9BsqtpQAAAABJRU5ErkJggg=='

vision_probe() {
  payload="$(printf '{\"model\":\"%s\",\"temperature\":0,\"max_tokens\":4,\"messages\":[{\"role\":\"user\",\"content\":[{\"type\":\"image_url\",\"image_url\":{\"url\":\"data:image/png;base64,%s\"}},{\"type\":\"text\",\"text\":\"Reply OK.\"}]}]}' "$model" "$probe_png")"
  url="${base_url%/}/chat/completions"

  if [ -n "$api_key" ]; then
    response="$(curl -fsS --connect-timeout 3 --max-time "$probe_timeout" \
      -H 'Content-Type: application/json' \
      -H "Authorization: Bearer $api_key" \
      --data "$payload" "$url" 2>/dev/null)" || return 1
  else
    response="$(curl -fsS --connect-timeout 3 --max-time "$probe_timeout" \
      -H 'Content-Type: application/json' \
      --data "$payload" "$url" 2>/dev/null)" || return 1
  fi

  printf '%s' "$response" | grep -q '"choices"'
}

wait_for_vision() {
  echo "GameVision: waiting for multimodal model '$model' at $base_url"
  until vision_probe; do
    echo "GameVision: vision probe failed; inference is unavailable or is not currently multimodal"
    sleep "$wait_interval"
  done
  echo "GameVision: vision probe succeeded"
}

case "${1:-}" in
  probe)
    vision_probe
    exit $?
    ;;
  wait-for-vision)
    wait_for_vision
    exit 0
    ;;
esac

if [ "${GAMEVISION_REQUIRE_VISION:-1}" != "0" ]; then
  wait_for_vision
fi

# Keep PID 1 as a small supervisor. If :8002 is switched back to the text-only
# model while GameVision is running, stop the game process. Swarm restarts this
# task and the next instance waits in preflight until vision is available again.
/usr/local/bin/gamevision "$@" &
child=$!

stop_child() {
  if kill -0 "$child" 2>/dev/null; then
    kill -TERM "$child" 2>/dev/null || true
  fi
  wait "$child" 2>/dev/null || true
}
trap 'stop_child; exit 0' INT TERM

if [ "${GAMEVISION_WATCH_VISION:-1}" != "0" ]; then
  while kill -0 "$child" 2>/dev/null; do
    sleep "$watch_interval"
    if ! kill -0 "$child" 2>/dev/null; then
      break
    fi
    if ! vision_probe; then
      echo "GameVision: multimodal inference disappeared; stopping until the vision model returns"
      stop_child
      exit 1
    fi
  done
fi

wait "$child"
