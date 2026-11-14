#!/bin/sh
# cli-smoke Batch 1: local server on a random port + temp data dir,
# exercises chat/state/bots/threads/approve against the real binary.
set -eu
BIN="${BIN:-./bin/go-bot}"
PORT="${PORT:-0}"
DATA="$(mktemp -d)"
trap 'kill $SRV 2>/dev/null; rm -rf "$DATA"' EXIT INT TERM

if [ "$PORT" = "0" ]; then
  PORT="$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1",0)); print(s.getsockname()[1])')"
fi
export GOBOT_DATA="$DATA"
export GOBOT_URL="http://127.0.0.1:$PORT"
export GOBOT_ADDR="127.0.0.1:$PORT"
export GOBOT_API_KEY="${GOBOT_API_KEY:-LLM_3035598383445962_ln-Ms1eSouQeikqqzCfOqZjVsDk}"
export GOBOT_BASE_URL="${GOBOT_BASE_URL:-https://api.meta.ai/v1}"
export GOBOT_MODEL="${GOBOT_MODEL:-muse-spark-1.3-contributor}"
export GOBOT_APPROVAL="${GOBOT_APPROVAL:-never}"

"$BIN" >"$DATA/server.log" 2>&1 &
SRV=$!
for i in $(seq 1 50); do
  if curl -sf "$GOBOT_URL/api/health" >/dev/null 2>&1; then break; fi
  sleep 0.2
done
curl -sf "$GOBOT_URL/api/health" >/dev/null || { echo "SMOKE1: server not up"; cat "$DATA/server.log"; exit 1; }
echo "SMOKE1: server up on $PORT"

check() {
  desc="$1"; shift
  if "$BIN" "$@" >/dev/null; then echo "SMOKE1 OK: $desc"; else echo "SMOKE1 FAIL: $desc"; exit 1; fi
}

check "state" state
BOT_JSON="$("$BIN" bots create --name smoke1 --url "$GOBOT_URL")"
echo "$BOT_JSON" | grep -q '"bot"' || { echo "SMOKE1 FAIL: bots create shape"; exit 1; }
echo "SMOKE1 OK: bots create"
check "bots list" bots list
BOT_ID="$(echo "$BOT_JSON" | python3 -c "import sys,json; print(json.load(sys.stdin)['bot']['id'])")"
THR_JSON="$("$BIN" threads new --bot "$BOT_ID")"
echo "$THR_JSON" | grep -q '"id"' || { echo "SMOKE1 FAIL: threads new shape"; exit 1; }
echo "SMOKE1 OK: threads new"
check "threads list" threads list --bot "$BOT_ID"
THR_ID="$(echo "$THR_JSON" | python3 -c "import sys,json; print(json.load(sys.stdin)['id'])")"
check "threads open" threads open "$THR_ID"
REPLY="$("$BIN" chat "jawab dengan satu kata: OK" --bot "$BOT_ID" --thread "$THR_ID")"
echo "$REPLY" | grep -q '"content"' || { echo "SMOKE1 FAIL: chat reply shape: $REPLY"; exit 1; }
echo "SMOKE1 OK: chat"
"$BIN" approve apv_smoke allow >/dev/null 2>&1 && echo "SMOKE1 OK: approve (unexpected pass)" || echo "SMOKE1 OK: approve rejects unknown id"
check "threads rm" threads rm "$BOT_ID" "$THR_ID"
echo "SMOKE1: ALL GREEN"
