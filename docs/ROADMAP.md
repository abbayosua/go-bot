# Roadmap

Go-Bot is an independent Go rewrite of the persistent-agent product shape: bots
that keep a conversation, a memory and a workspace, run on their own schedule,
and ask a person only when they must.

This file is the plan. A checkbox is done only when the behaviour works and a
test covers it. Bracketed numbers map to the feature-gap list agreed against the
reference product, so nothing agreed is silently dropped.

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

- [#14] **Graphical computer and screen takeover** (X display, Chrome profiles,
  VNC, computer-use actions). Revisit only if a task cannot be done with a
  shell, a file and an HTTP call.
- [#15] **Sandbox multi-providers and browser automation** (Docker, E2B, Daytona,
  CreateOS, Boat, Boxd, `browser_navigate` and friends). Tools run on the host
  instead. Revisit only if multi-tenant execution is needed.

Consequence to keep in mind: Go-Bot is trusted with the machine it runs on. See
Safety in `README.md`.

## Stage 1 — a daily driver

Goal: one bot can hold a long project without losing the thread.

- [ ] [#8] **Tool activity feed.** Each tool call becomes a compact row in the chat
      (name, one-line summary, expandable output) instead of a raw blob.
      Files: `internal/httpapi`, `web/app.js`. Done when a shell failure and a
      file write both render as one row and expand.
- [ ] [#1, #3] **Memory.** `remember` and `recall` tools; memory per bot, plus an
      optional shared install-wide scope. Injected into the system prompt as a
      bounded, newest-first block.
      Files: `internal/store` (new table), `internal/tools`, `internal/agent`.
      Done when a fact written in one turn is available in a later turn and the
      injection stays under a fixed token budget.
- [ ] [#2] **External memory providers.** One neutral `MemoryProvider` interface;
      an external service is an adapter behind it, never a special code path.
      Files: `internal/core` (new package), `internal/agent`. Done when a second
      provider can be added without touching the agent loop.
- [ ] [#4] **Scratchpad.** A per-bot working-notes area, readable mid-run, kept
      out of long-term memory.
      Files: `internal/store`, `internal/agent`. Done when notes written today
      guide tomorrow's run and can be cleared in one action.
- [ ] [#5] **Knowledge library.** Uploaded and bot-written pages the agent can
      search before it answers.
      Files: `internal/store`, `internal/agent`, `internal/httpapi`, `web`.
      Done when a bot cites a library page in an answer.
- [ ] [#6] **Context compaction.** When history exceeds a budget, older turns
      collapse into a summary stored as its own row, and the model keeps working.
      Files: `internal/agent`. Done when a 200-message thread still answers with
      the original task in view and the budget is enforced by a test.
- [ ] [#7] **Pre-flight checks.** A risky instruction or tool plan is tested
      against a policy gate before it runs.
      Files: `internal/agent`. Done when a destructive plan is stopped with a
      reason the user can read.
- [ ] **Instructions from chat.** The model can propose an instruction change; the
      UI shows it as a card with Undo; every version is kept.
      Files: `internal/store` (version table), `internal/agent`, `internal/httpapi`.
      Done when a change is visible, auditable and reversible.
- [ ] [#26] **Skills.** Markdown playbooks in a shared library, invocable with
      `/name`.
      Files: `internal/store`, `internal/agent`, `internal/httpapi`.
      Done when a skill is listed, invoked, and its body reaches the prompt.
- [ ] [#9] **Durable approval rules.** `Always allow this tool` and `Always allow this
      site`, editable in settings, checked before the gate asks.
      Files: `internal/store`, `internal/agent`, `internal/httpapi`, `web`.
      Done when a rule survives a restart and a denied rule never opens a card.
- [ ] [#11] **Run cancel.** The UI can stop a running turn; the run ends cleanly and the
      partial output is kept.
- [ ] [#12] **Artifacts.** Files the model produces (documents, images, archives)
      become openable objects in the chat, not just paths in text.
      Files: `internal/store`, `internal/httpapi`, `web`. Done when an artifact
      opens from the chat and downloads intact.
- [ ] [#13] **Encrypted attachments.** Uploads are encrypted at rest and served
      only to the asking session.
      Files: `internal/store`, `internal/httpapi`. Done when the database file
      alone cannot reveal an attachment.

## Stage 2 — more than one bot

- [ ] **Spaces.** A space owns bots, threads, memory and a workspace root.
      Everything scoped by `space_id`, with the same isolation guarantees the
      reference product documents.
- [ ] **Groups.** Two to ten bots in one chat, each keeping its own memory and
      workspace; mentions route the turn.
- [ ] **Delegation.** A bot can hand a task to a peer bot or a short-lived
      subagent and read the result back. Bounded by depth and count. [#19]
- [ ] **Fleet tools.** A bot can create, rename, instruct and schedule other bots,
      behind a per-bot switch. [#20]
- [ ] [#18] **Bot-to-bot messages.** Bots DM each other with a readable handoff;
      every cross-bot claim is cited back to its source.
      Files: `internal/store`, `internal/agent`. Done when bot A answers using
      information fetched through bot B.
- [ ] [#21] **Space search.** One search across every bot and thread in a space.
      Files: `internal/store`, `internal/httpapi`, `web`. Done when a query
      finds messages from two different bots.
- [ ] [#22] **Share links.** A chat or artifact shared as a revocable link.
      Files: `internal/store`, `internal/httpapi`. Done when a revoked link
      stops working immediately.

## Stage 3 — scheduled and triggered autonomy

- [ ] **Routine editor and history.** Past run results per routine; a schedule
      editor with preview. [#25]
- [ ] [#23] **Watch triggers.** A routine fires when a watched file changes or a
      watched command changes output.
      Files: `internal/schedule`, `internal/store`, `internal/httpapi`.
      Done when editing a watched file starts the run and the run sees the diff.
- [ ] [#24] **Inbound triggers.** A signed webhook, a Git push or a chat message
      can start a named routine.
      Files: `internal/httpapi`, `internal/schedule`. Done when an unsigned
      request is rejected and a signed one delivers its payload to the prompt.

## Stage 4 — the outside world

- [ ] [#28] **MCP client.** stdio and streamable HTTP, with per-server approval
      and health reporting.
- [ ] **Connector interface.** One neutral contract; managed catalogs (if ever) are
      adapters behind it, never the shape of the API. [#29]
- [ ] [#39, #40] **Secrets.** Encrypted at rest, requested by the model through a
      card, never echoed back in tool output or logs.
- [ ] [#32] **Messaging.** One adapter first (Telegram), then the rest. Inbound
      text is treated as untrusted input at the approval gate.
- [ ] [#33] **Messaging installations.** Connection list per bot and a share list
      of who may talk to it.
      Files: `internal/store`, `internal/httpapi`, `web`. Done when removing a
      person stops their messages from reaching the bot.
- [ ] [#27] **Plugins.** A plugin is a folder with a manifest and tools; install,
      enable and disable from the UI.
      Files: `internal/tools`, `internal/store`, `internal/httpapi`, `web`.
      Done when installing a folder adds a working tool to one bot only.
- [ ] [#30] **App connections.** OAuth to outside services (Google, GitHub, ...)
      per bot, tokens encrypted with the secrets system.
      Files: `internal/store`, `internal/httpapi`, `web`. Done when a token
      refresh needs no user action.
- [ ] [#31] **Provider check.** Before a model is trusted, verify it can stream,
      call tools and return images in the shape the loop needs.
      Files: `internal/llm`, `internal/httpapi`. Done when an incompatible
      endpoint fails setup with a named reason.

## Stage 5 — accounts and operations

- [ ] [#35] **Multi-user.** Accounts, sessions, per-space roles, row-level scoping.
- [ ] [#36] **Auth hardening.** Passkeys, upstream OIDC, password policy, session
      list with remote sign-out.
- [ ] [#10] **Audit log.** Every privileged action recorded and exportable.
- [ ] [#37] **Onboarding.** A guided first run that ends with a working bot, not
      an empty sidebar.
- [ ] [#38] **Model roles and fallback.** Different models per role (chat, judge,
      cheap); a failed call retries on the next configured model in the same role.
- [ ] [#41] **Payment wallet.** A balance the agent draws from for paid actions,
      with per-action caps and receipts.
- [ ] [#42] **Self-host kit.** Compose file, backup command, restore drill,
      restricted-network guide.
- [ ] [#51] **Observability.** Structured run timings, a queue-delay signal, a
      health endpoint, optional OTLP export.
- [ ] **Export/import.** One archive with bots, memory, skills and settings.

## Stage 6 — surfaces

- [ ] [#43] **Multi-panel shell.** Resizable sidebar, settings overlays, deep-
      linked pages for every settings area.
- [ ] [#44] **Boards.** A kanban a bot keeps for its own tasks.
- [ ] [#45] **Admin panel.** Users, spaces, providers and plugins in one place.
- [ ] [#46] **Agent import.** Turn another assistant's folder (instructions,
      memory, skills) into a new bot next to the existing ones.
- [ ] [#47] **Interface languages.** Full i18n for the shipped strings.
- [ ] [#50] **External agents.** Delegate a task to an outside agent service and
      read the result back like a subagent.
- [ ] [#34] **Voice.** Dictation first, then spoken replies.
- [ ] **PWA.** Installable, offline shell, push for approvals.
- [ ] **Mobile.** Native client against the same API; no forked logic.
- [ ] **Desktop.** Thin native shell around the same UI.

## Order rationale

Stage 1 makes one bot worth using daily. Stage 2 makes it a team. Stage 3 makes
it act without being asked. Stage 4 makes it useful beyond files. Stages 5 and 6
assume other people will run it.

Anything in Not in scope is not a prerequisite for any stage above.
