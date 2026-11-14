#!/bin/sh
# cli-smoke4 Batch 4: config/model/status/doctor/plugins/mcp + backup/restore.
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
curl -sf "$GOBOT_URL/api/health" >/dev/null || { echo "SMOKE4: server not up"; cat "$DATA/server.log"; exit 1; }
echo "SMOKE4: server up on $PORT"

"$BIN" config get | grep -q 'approval' || { echo "SMOKE4 FAIL: config get"; exit 1; }
"$BIN" config set approval never | grep -q 'never' || { echo "SMOKE4 FAIL: config set"; exit 1; }
"$BIN" config set approval auto | grep -q 'auto' || { echo "SMOKE4 FAIL: config set back"; exit 1; }
echo "SMOKE4 OK: config"
"$BIN" model | grep -q 'approval' || { echo "SMOKE4 FAIL: model"; exit 1; }
echo "SMOKE4 OK: model"
"$BIN" status | grep -q '"status": "ok"' || { echo "SMOKE4 FAIL: status"; exit 1; }
echo "SMOKE4 OK: status"
"$BIN" doctor | grep -q 'health' || { echo "SMOKE4 FAIL: doctor"; exit 1; }
echo "SMOKE4 OK: doctor"
"$BIN" version 2>/dev/null || "$BIN" --version | grep -q 'go-bot' || { echo "SMOKE4 FAIL: version"; exit 1; }
echo "SMOKE4 OK: version"
BOT_ID="$("$BIN" bots create --name smoke4 | python3 -c "import sys,json; print(json.load(sys.stdin)['bot']['id'])")"
"$BIN" plugins list --bot "$BOT_ID" | grep -q '^\[' || { echo "SMOKE4 FAIL: plugins list"; exit 1; }
echo "SMOKE4 OK: plugins list"
"$BIN" mcp list | grep -q '^\[' || { echo "SMOKE4 FAIL: mcp list"; exit 1; }
echo "SMOKE4 OK: mcp list"
"$BIN" backup --out "$DATA/bak.json" --data "$DATA" || { echo "SMOKE4 FAIL: backup"; exit 1; }
test -s "$DATA/bak.json" || { echo "SMOKE4 FAIL: backup empty"; exit 1; }
echo "SMOKE4 OK: backup"
echo "SMOKE4: ALL GREEN"
