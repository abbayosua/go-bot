# Roadmap

Go-Bot is an independent Go rewrite of the persistent-agent product shape: bots
that keep a conversation, a memory and a workspace, run on their own schedule,
and ask a person only when they must.

This file is the plan. A checkbox is done only when the behaviour works and a
test covers it.

## Principles

- One binary, one SQLite file, one workspace per bot. No Postgres, no broker, no
  Electron, no required container.
- The message history is the run checkpoint. Any pause must be resumable from it.
- Every capability lands behind an interface in `internal/`, never inside the
  HTTP layer.
- New providers and integrations are optional at runtime: absent config means the
  feature is invisible, not broken.
- UI copy stays minimal. If a control can be obvious without a sentence, drop the
  sentence.

## Shipped

- [x] Single binary with the UI embedded (`web/`, `web/embed.go`)
- [x] SQLite schema and queries (`internal/store`)
- [x] Streaming OpenAI-compatible client with tool calls (`internal/llm`)
- [x] Tools: `shell`, `read_file`, `write_file`, `list_files`, `web_fetch` (`internal/tools`)
- [x] Workspace sandboxing: `..`, absolute paths and symlink escapes refused
- [x] Run loop with lease claim, loop guard and step limit (`internal/agent`)
- [x] Approvals `ask` / `auto` / `never`, resumable from the message history
- [x] Cron routines with a ticker (`internal/schedule`)
- [x] Server-sent events for live runs (`internal/httpapi`)
- [x] Bearer token required off loopback (`internal/config`)

## Not in scope

These are deliberately deferred. They are the largest and least portable parts of
the reference product, and they do not serve a single-user local agent.

- **Sandbox providers** (Docker, E2B, Daytona, Boat, hosted desktops). Tools run
  on the host instead. Revisit only if multi-tenant execution is needed.
- **Graphical desktop and screen takeover** (X display, Chrome profiles, VNC,
  computer-use actions). Revisit only if a task cannot be done with a shell, a
  file and an HTTP call.
- **Browser automation and vision-driven clicking.** `web_fetch` covers reading;
  headless automation is a separate product decision.

Consequence to keep in mind: Go-Bot is trusted with the machine it runs on. See
Safety in `README.md`.

## Stage 1 — a daily driver

Goal: one bot can hold a long project without losing the thread.

- [ ] **Tool activity feed.** Each tool call becomes a compact row in the chat
      (name, one-line summary, expandable output) instead of a raw blob.
      Files: `internal/httpapi`, `web/app.js`. Done when a shell failure and a
      file write both render as one row and expand.
- [ ] **Memory.** `remember` and `recall` tools; memory per bot, plus an optional
      shared scope for the whole install. Injected into the system prompt as a
      bounded, newest-first block.
      Files: `internal/store` (new table), `internal/tools`, `internal/agent`.
      Done when a fact written in one turn is available in a later turn and the
      injection stays under a fixed token budget.
- [ ] **Instructions from chat.** The model can propose an instruction change; the
      UI shows it as a card with Undo; every version is kept.
      Files: `internal/store` (version table), `internal/agent`, `internal/httpapi`.
      Done when a change is visible, auditable and reversible.
- [ ] **Context compaction.** When history exceeds a budget, older turns collapse
      into a summary stored as its own row, and the model keeps working.
      Files: `internal/agent`. Done when a 200-message thread still answers with
      the original task in view and the budget is enforced by a test.
- [ ] **Skills.** Markdown playbooks in a shared library, invocable with `/name`.
      Files: `internal/store`, `internal/agent`, `internal/httpapi`.
      Done when a skill is listed, invoked, and its body reaches the prompt.
- [ ] **Durable approval rules.** `Always allow this tool` and `Always allow this
      site`, editable in settings, checked before the gate asks.
      Files: `internal/store`, `internal/agent`, `internal/httpapi`, `web`.
      Done when a rule survives a restart and a denied rule never opens a card.
- [ ] **Run cancel.** The UI can stop a running turn; the run ends cleanly and the
      partial output is kept.

## Stage 2 — more than one bot

- [ ] **Spaces.** A space owns bots, threads, memory and a workspace root.
      Everything scoped by `space_id`, with the same isolation guarantees the
      reference product documents.
- [ ] **Groups.** Two to ten bots in one chat, each keeping its own memory and
      workspace; mentions route the turn.
- [ ] **Delegation.** A bot can hand a task to a peer bot or a short-lived
      subagent and read the result back. Bounded by depth and count.
- [ ] **Fleet tools.** A bot can create, rename, instruct and schedule other bots,
      behind a per-bot switch.

## Stage 3 — the outside world

- [ ] **MCP client.** stdio and streamable HTTP, with per-server approval and
      health reporting.
- [ ] **Connector interface.** One neutral contract; managed catalogs (if ever) are
      adapters behind it, never the shape of the API.
- [ ] **Secrets.** Encrypted at rest, requested by the model through a card,
      never echoed back in tool output or logs.
- [ ] **Messaging.** One adapter first (Telegram), then the rest. Inbound text is
      treated as untrusted input at the approval gate.

## Stage 4 — accounts and operations

- [ ] **Multi-user.** Accounts, sessions, per-space roles, row-level scoping.
- [ ] **Auth hardening.** Passkeys, upstream OIDC, password policy, session list.
- [ ] **Audit log.** Every privileged action recorded, exportable.
- [ ] **Observability.** Structured run timings, queue delay signal, health
      endpoint, optional OTLP export.
- [ ] **Export/import.** One archive with bots, memory, skills and settings.

## Stage 5 — surfaces

- [ ] **PWA.** Installable, offline shell, push for approvals.
- [ ] **Mobile.** Native client against the same API; no forked logic.
- [ ] **Desktop.** Thin native shell around the same UI.
- [ ] **Voice.** Dictation first, then spoken replies.

## Order rationale

Stage 1 makes one bot worth using daily. Stage 2 makes it a team. Stage 3 makes it
useful beyond files. Stages 4 and 5 assume other people will run it.

Anything in Not in scope is not a prerequisite for any stage above.
