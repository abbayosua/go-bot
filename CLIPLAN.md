# CLIPLAN — CLI Go-Bot per Batch (cermin Hermes, gateway hanya Telegram)

Sumber: `hermes_cli/subcommands/` (±40 subcommand Hermes), ±90 route
`internal/httpapi/server.go` Go-Bot. Discord/WA/Weixin DIHILANGKAN.

## Batch 1 — inti chat
- `chat "pesan" [--bot] [--thread]` (SSE streaming ke thread, cetak balasan)
- `state` (status server, bot/thread aktif, usage cache)
- `bots list/create/rm`
- `threads list/new/rm/open`
- `approve <id> allow|deny`

## Batch 2 — panel data
- `search "q"`
- `audit [--csv]`
- `memory list/forget`
- `notes add/list/clear`
- `board list/add/move/del`

## Batch 3 — otomasi
- `cron list/add/edit/pause/resume/run/del` (cermin Hermes `cron`; `routines` alias)
- `skills list/add/rm`
- `rules list/add/rm`

## Batch 4 — sistem
- `config get/set`
- `model`
- `status/doctor`
- `version`
- `backup/restore` (rapikan existing `cmd/go-bot/backup.go`)
- `plugins/mcp` (list/install/enable)

## Batch 5 — Telegram gateway saja
- `gateway setup --telegram-token ...`
- `gateway run` (long-poll Telegram -> teruskan ke thread bot, balasan kembali)
- Tanpa Discord/WA/Weixin.

## Verifikasi tiap batch
`go vet ./...` + `go test ./...` + uji CLI nyata (`./bin/go-bot <cmd>`)
+ commit + push.

## Testing mandiri (wajib tiap batch, tanpa server luar)
1. `go test ./internal/...` + `go test ./cmd/...` untuk tiap subcommand baru
   (table-driven: parse args, panggil store/API lokal, assert output).
2. Skrip `scripts/cli-smoke.sh`: start server lokal di port acak + data
   sementara (`GOBOT_DATA=$(mktemp -d)`), jalankan tiap subcommand batch
   terkait, assert exit 0 + output JSON valid, lalu matikan server.
3. Batch tidak dianggap selesai sebelum smoke script batch itu hijau.
