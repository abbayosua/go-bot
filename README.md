# Go-Bot

An autonomous agent you run yourself. One Go binary, one SQLite file, one
workspace per bot. No Python, no Node, no Postgres, no Electron.

This is an independent rewrite, not a fork: every line is new Go, and no code
was copied from any other project.

## What it does

- **Bots** with their own instructions, model and workspace.
- **Runs** that work until the task is done, are reflected in the chat live,
  and are claimed with a lease so two workers never run one turn twice.
- **Tools**: `shell`, `read_file`, `write_file`, `list_files`, `web_fetch`.
- **Approvals**: `ask`, `auto` (a judge model decides) or `never`. The message
  history is the checkpoint, so a paused run resumes exactly where it stopped.
- **Routines**: cron-scheduled prompts that start with nobody watching.
- **Loop guard**: a model repeating one call stops instead of burning tokens.

## Run it

```bash
go run ./cmd/go-bot -api-key "sk-..." -model gpt-4.1-mini
```

Open http://127.0.0.1:7417. A first bot is created for you.

## Configure

Flags fall back to environment variables.

| Flag | Env | Default |
| --- | --- | --- |
| `-addr` | `GOBOT_ADDR` | `127.0.0.1:7417` |
| `-data` | `GOBOT_DATA` | `~/.go-bot` |
| `-token` | `GOBOT_TOKEN` | none; required off loopback |
| `-approval` | `GOBOT_APPROVAL` | `ask` |
| `-base-url` | `GOBOT_BASE_URL` | `https://api.openai.com/v1` |
| `-api-key` | `GOBOT_API_KEY` | none |
| `-model` | `GOBOT_MODEL` | `gpt-4.1-mini` |
| `-judge-model` | `GOBOT_JUDGE_MODEL` | none (auto then asks) |

Any OpenAI-compatible endpoint works: a local server, a gateway, or a hosted
API. `-base-url` must point at the `/v1` root.

## Layout

```
cmd/go-bot            flags, wiring, graceful shutdown
internal/config     flags over environment, validation
internal/store      SQLite schema and queries (bots, threads, messages, runs, approvals, routines)
internal/llm        streaming chat-completions client with tool calls
internal/tools      sandboxed shell, files and web fetch
internal/agent      the run loop: model, tools, approvals, resume, loop guard
internal/schedule   five-field cron and the routine ticker
internal/httpapi    JSON API, server-sent events, embedded UI
web                 single-page UI, compiled into the binary
```

## How a run works

```
message -> run row -> engine claims a lease -> model streams -> tool calls
        -> approval gate -> tool results -> model again -> done
```

Nothing sits in a queue between a message and its run. The UI follows the run
over `GET /api/threads/{id}/events`; a dropped stream reconnects and reloads
from the message history.

A run pauses in three places: an approval, a cancellation, or the step limit.
All three are recoverable, because the messages themselves are the state.

## Safety

Go-Bot runs on your machine, as you. That shapes the defaults:

- The API is bound to loopback and refuses a public address without a token.
- File tools resolve paths against the workspace and refuse escapes, including
  through symlinks.
- `shell` refuses a short list of destructive commands (`rm -rf /`, `mkfs`,
  `sudo`, writing to block devices, fork bombs, shutdown) in every approval
  mode and returns the refusal to the model, which can route around it.
- Approval mode `never` is real: no prompts, every tool runs. Choose it only
  where you would run the command yourself.
- The workspace is a directory, not a container. For real isolation, run Go-Bot
  as a user that owns only that directory, or inside a container.

## Development

```bash
make test     # go test ./...
make vet      # go vet ./...
make build    # bin/go-bot
make run      # go run ./cmd/go-bot
```

Tests are offline: a fake provider serves server-sent events, so no model call
and no network are needed.

## Not built yet

Deliberately out of scope for this cut: multi-user accounts, remote sandboxes,
memory providers, connectors, voice, and mobile clients. The seams for them
are `internal/store`, `internal/tools` and `internal/agent`.
