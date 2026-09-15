#!/bin/sh
set -eu

base_url="${GAMEVISION_BASE_URL:-http://192.168.50.81:8002/v1}"
model="${GAMEVISION_MODEL:-Qwen3-VL-4B-Instruct}"
api_key="${GAMEVISION_API_KEY:-}"
probe_timeout="${GAMEVISION_VISION_PROBE_TIMEOUT:-30}"
wait_interval="${GAMEVISION_VISION_WAIT_INTERVAL:-10}"
# 1x1 PNG. A text-only llama-server started without mmproj rejects this
# multimodal request; a working vision endpoint accepts it.
probe_png='iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII='

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
    echo "GameVision: vision probe failed; :8002 is unavailable or is not currently serving the configured vision model"
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

exec /usr/local/bin/gamevision "$@"
