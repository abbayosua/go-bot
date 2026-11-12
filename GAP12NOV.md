# GAP12NOV — Audit Paritas Vanilla (`73bd2d5`) vs Lit

Tanggal: 2026-11-12. Sumber fakta: `/tmp/legacy-index.html` (339 baris, 34 ID panel/form/list),
`/tmp/legacy-app.js` (1698 baris), komponen Lit `web/components/*.js`, mockup
`/Users/user/Downloads/claudegobot.html`, verifikasi CDP + `node --check` + `go vet` + `go test`.

## Yang sudah dikerjakan (12NOV1536, commit 29fad58 + b2eb0e2)

UI Lit (`g-app` + `g-tool-drawer`) hanya memproyeksikan teks deskripsi per slot.
12 panel fungsional vanilla masih ada di git (`73bd2d5:web/index.html` +
`73bd2d5:web/app.js`) tapi tak ter-mount. Yang dilakukan:

1. File baru `web/components/g-panels.js` (470 baris, 12 komponen
   `g-panel-search/audit/admin/plugins/board/backup/routines/memory/notes/skills/rules/settings`),
   tiap panel fetch API backend nyata.
2. Wire 12 light-DOM ke slot drawer di `web/components/g-app.js` + `.botId`/`.api`;
   toolbar quick Search/Memory/Skills + popover More Tools 12 ala mockup.
3. Perbaikan kontrak (fakta backend, bukan asumsi):
   - `web/embed.go`: embed per-file -> direktori (fix `g-panels.js` 404).
   - Plugins `POST {path}` (bukan `name`); enable `PUT /api/plugins/{id}`.
   - Rules `POST {scope,kind,tool,args}` (bukan `{text}`); list `?botId=`.
   - Settings simpan via `PATCH /api/bots/{id}`; fleet `PUT .../fleet {fleet}`.
   - Backup info size dari `blob.size` (header `content-length` 0 karena chunked);
     restore file via `POST /api/backup`.
4. New Chat mati (cuma toast `g-action`) -> hidup via `POST /api/bots/{id}/threads`
   + `handleAction(new-chat/new-space/new-group/settings)` (commit `b2eb0e2`).

Verifikasi: tiap panel dibuka via CDP tab fresh (unregister SW `gobot-v5`),
tulis-baca round-trip konsisten, screenshot cocok (`/tmp/shot-*.jpg`);
`node --check` 4 file OK, `go vet` clean, `go test` 13 paket `ok`.
Screenshot final vs mockup Claude verdict COCOK.

## Paritas tercapai

- Chat inti: kirim/Enter/streaming SSE, markdown, tool row, approval card,
  artifact inline + panel preview/code/copy/open/reload.
- 12 panel drawer: semua CRUD API jalan, CDP verified.
- Sidebar roster/spaces/footer, New Chat (thread baru), tema Claude/plain,
  toast, version badge, cache `⚡%` indicator.

## Gap tersisa (vanilla ada, Lit belum)

| # | Fitur vanilla (ID/fungsi legacy) | Status Lit | Dampak |
|---|---|---|---|
| 1 | `new-bot-dialog` + `new-bot-form` — buat **bot baru** (nama+instruksi) via `POST /api/bots` | New Chat = thread baru di bot aktif | Tidak bisa tambah bot dari UI |
| 2 | `delete-thread` (`DELETE /api/bots/{bot}/threads/{thread}`) + `confirm()` | Tak ada | Thread menumpuk, tak bisa hapus |
| 3 | `space-select` onchange + `new-space-dialog` + `delete-space` (`DELETE /api/spaces/{id}`) + `new-group-dialog` | `prompt()` kasar, tak bisa hapus space | UX jelek |
| 4 | `attach` upload (`POST .../attachments`) + `attachments` list + `speak` TTS + `mic` dictate | Composer cuma teks + tombol hiasan | File/voice mati |
| 5 | `approval` select -> `PUT /api/settings` | Sidebar tampilkan saja | Mode approval tak bisa diubah |
| 6 | `lang` select i18n (`web/i18n.js`, `t()`) + `renderSpaces/renderRoster` detail | Tak ada | Minor |
| 7 | Deep-link `#/routines` dkk + `openPanel` | Tak ada | Tak bisa link langsung ke panel |
| 8 | `qt-board`/`qt-bot` quick-tools | Lit cuma Search/Memory/Skills + More Tools | 2 shortcut hilang |
| 9 | Panel tak auto-reload saat drawer dibuka ulang (contoh: Board) | Dikonfirmasi CDP step 7 | Data basi sampai refresh manual |

## Usulan eksekusi

Batch A (fungsional, prioritas): gap 1-5.
Batch B (kosmetik): gap 6-9.
