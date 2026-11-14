#!/bin/sh
# cli-smoke2 Batch 2: search/audit/memory/notes/board against a throwaway server.
set -eu
BIN="${BIN:-./bin/go-bot}"
DATA="$(mktemp -d)"
trap 'kill $SRV 2>/dev/null; rm -rf "$DATA"' EXIT INT TERM

PORT="$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1",0)); print(s.getsockname()[1])')"
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
curl -sf "$GOBOT_URL/api/health" >/dev/null || { echo "SMOKE2: server not up"; cat "$DATA/server.log"; exit 1; }
echo "SMOKE2: server up on $PORT"
BOT_ID="$("$BIN" bots create --name smoke2 | python3 -c "import sys,json; print(json.load(sys.stdin)['bot']['id'])")"
echo "SMOKE2: bot $BOT_ID"

"$BIN" search "smoke" --limit 3 | grep -q '^\[' || { echo "SMOKE2 FAIL: search"; exit 1; }
echo "SMOKE2 OK: search"
"$BIN" audit --limit 3 | grep -q '^\[' || { echo "SMOKE2 FAIL: audit json"; exit 1; }
echo "SMOKE2 OK: audit json"
"$BIN" audit --limit 3 --csv | grep -q 'id' || { echo "SMOKE2 FAIL: audit csv"; exit 1; }
echo "SMOKE2 OK: audit csv"
"$BIN" memory list --bot "$BOT_ID" | grep -q '^\[' || { echo "SMOKE2 FAIL: memory list"; exit 1; }
echo "SMOKE2 OK: memory list"
"$BIN" notes add "smoke-note-2" --bot "$BOT_ID" | grep -q 'noted' || { echo "SMOKE2 FAIL: notes add"; exit 1; }
"$BIN" notes list --bot "$BOT_ID" | grep -q 'smoke-note-2' || { echo "SMOKE2 FAIL: notes list"; exit 1; }
"$BIN" notes clear --bot "$BOT_ID" | grep -q 'cleared' || { echo "SMOKE2 FAIL: notes clear"; exit 1; }
echo "SMOKE2 OK: notes round-trip"
CARD="$("$BIN" board add "smoke-card-2" --bot "$BOT_ID")"
CARD_ID="$(echo "$CARD" | python3 -c "import sys,json; print(json.load(sys.stdin)['id'])")"
"$BIN" board move "$CARD_ID" doing | grep -q 'doing' || { echo "SMOKE2 FAIL: board move"; exit 1; }
"$BIN" board del "$CARD_ID" | grep -q 'deleted' || { echo "SMOKE2 FAIL: board del"; exit 1; }
"$BIN" board list --bot "$BOT_ID" | grep -q '^\[' || { echo "SMOKE2 FAIL: board list"; exit 1; }
echo "SMOKE2 OK: board round-trip"
echo "SMOKE2: ALL GREEN"
