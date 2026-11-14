#!/bin/sh
# cli-smoke3 Batch 3: cron/skills/rules against a throwaway server.
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
curl -sf "$GOBOT_URL/api/health" >/dev/null || { echo "SMOKE3: server not up"; cat "$DATA/server.log"; exit 1; }
echo "SMOKE3: server up on $PORT"
BOT_ID="$("$BIN" bots create --name smoke3 | python3 -c "import sys,json; print(json.load(sys.stdin)['bot']['id'])")"

R="$("$BIN" cron add --cron "0 9 * * *" --bot "$BOT_ID" "smoke routine")"
RID="$(echo "$R" | python3 -c "import sys,json; print(json.load(sys.stdin)['id'])")"
"$BIN" cron list --bot "$BOT_ID" | grep -q "$RID" || { echo "SMOKE3 FAIL: cron list"; exit 1; }
"$BIN" cron edit "$RID" --prompt "smoke routine v2" | grep -q 'v2' || { echo "SMOKE3 FAIL: cron edit"; exit 1; }
"$BIN" cron pause "$RID" | grep -q 'false' || { echo "SMOKE3 FAIL: cron pause"; exit 1; }
"$BIN" cron resume "$RID" | grep -q 'true' || { echo "SMOKE3 FAIL: cron resume"; exit 1; }
"$BIN" cron del "$RID" | grep -q 'deleted' || { echo "SMOKE3 FAIL: cron del"; exit 1; }
echo "SMOKE3 OK: cron round-trip"
"$BIN" routines list --bot "$BOT_ID" | grep -q '^\[' || { echo "SMOKE3 FAIL: routines alias"; exit 1; }
echo "SMOKE3 OK: routines alias"

S="$("$BIN" skills add --name "smoke-skill-3" --body "do x")"
SID="$(echo "$S" | python3 -c "import sys,json; print(json.load(sys.stdin)['id'])")"
"$BIN" skills list | grep -q "$SID" || { echo "SMOKE3 FAIL: skills list"; exit 1; }
"$BIN" skills rm "$SID" | grep -q 'deleted' || { echo "SMOKE3 FAIL: skills rm"; exit 1; }
echo "SMOKE3 OK: skills round-trip"

U="$("$BIN" rules add --kind allow --tool "smoke-tool-3")"
RID2="$(echo "$U" | python3 -c "import sys,json; print(json.load(sys.stdin)['id'])")"
"$BIN" rules list | grep -q "$RID2" || { echo "SMOKE3 FAIL: rules list"; exit 1; }
"$BIN" rules rm "$RID2" | grep -q 'deleted' || { echo "SMOKE3 FAIL: rules rm"; exit 1; }
echo "SMOKE3 OK: rules round-trip"
echo "SMOKE3: ALL GREEN"
