# Restricted networks

Go-Bot needs exactly one egress: the model provider's HTTPS endpoint.

1. Allow `https://api.openai.com` (or your `GOBOT_BASE_URL` host), port 443.
2. Deny everything else egress. The binary makes no other outbound calls
   except Telegram/MCP/plugin URLs you configure yourself.
3. Run behind loopback or a reverse proxy with TLS; set `GOBOT_TOKEN` so the
   API requires a bearer off loopback.
4. Mount `/data` on an encrypted volume; backups (`go-bot backup`) contain
   the SQLite file, so encrypt the backup target too.

Offline use: point `GOBOT_BASE_URL` at a local OpenAI-compatible server on
the same host. No other feature needs the internet.
