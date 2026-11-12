import { t, setLang, currentLang } from "./i18n.js";

const state = {
  token: localStorage.getItem("gobot_token") || "",
  bots: [],
  spaces: [],
  spaceId: "",
  approval: "ask",
  botId: "",
  threadId: "",
  messages: [],
  approvals: [],
  runs: [],
  stream: null,
  streaming: null,
  usage: null,
  previewPath: "",
  previewMode: "preview",
  artifactFiles: [],
};

const el = (id) => {
  const direct = document.getElementById(id);
  if (direct) return direct;
  const aliased = document.querySelector(`[data-legacy="${id}"]`);
  if (aliased) return aliased;
  // Notes and Bot settings reuse existing panels under their own names.
  if (id === "notes") return document.getElementById("scratchpad-panel");
  if (id === "bot") return document.getElementById("settings-panel");
  return null;
};

function legacyAlias(newId, oldId) {
  const target = document.getElementById(newId);
  if (target) target.dataset.legacy = oldId;
}

function aliasPanels() {
  for (const name of ["routines", "memory", "skills", "rules", "settings", "search", "audit", "backup", "board", "admin", "plugins", "scratchpad"]) {
    legacyAlias(`${name}-panel`, name);
  }
}

async function api(path, options = {}) {
  const headers = { ...(options.headers || {}) };
  if (!(options.body instanceof FormData) && !headers["Content-Type"]) {
    headers["Content-Type"] = "application/json";
  }
  if (state.token) headers.Authorization = `Bearer ${state.token}`;
  const response = await fetch(path, { ...options, headers });
  if (response.status === 401) {
    const provided = prompt("Access token for this Go-Bot instance:");
    if (provided) {
      state.token = provided;
      localStorage.setItem("gobot_token", provided);
      return api(path, options);
    }
  }
  if (!response.ok) {
    const body = await response.json().catch(() => ({}));
    throw new Error(body.error || `request failed (${response.status})`);
  }
  if (response.status === 204) return null;
  return response.json();
}

function currentBot() {
  return state.bots.find((bot) => bot.id === state.botId);
}

function activeRun() {
  return state.runs.find((run) => ["queued", "running", "waiting_approval"].includes(run.status));
}

async function loadState() {
  try {
    const health = await api("/api/health");
    if (health.version) el("app-version").textContent = health.version.startsWith("v") ? health.version : `v${health.version}`;
  } catch (error) {
    el("app-version").textContent = "";
  }
  const data = await api(state.spaceId ? `/api/state?spaceId=${state.spaceId}` : "/api/state");
  state.bots = data.bots;
  state.spaces = data.spaces || [];
  state.approval = data.approval;
  el("approval").value = data.approval;
  renderSpaces();
  if (state.spaceId && !state.spaces.some((space) => space.id === state.spaceId)) {
    state.spaceId = "";
  }
  if (!state.bots.length) {
    state.botId = "";
    state.threadId = "";
    state.messages = [];
    renderRoster();
    renderThread();
    return;
  }
  if (!state.bots.some((bot) => bot.id === state.botId)) {
    state.botId = state.bots[0].id;
  }
  const bot = currentBot();
  if (!bot.threads.some((thread) => thread.id === state.threadId)) {
    state.threadId = bot.threads[0]?.id || "";
  }
  renderRoster();
  await loadThread();
}

async function loadThread() {
  if (!state.threadId) {
    state.messages = [];
    state.approvals = [];
    state.runs = [];
    renderThread();
    return;
  }
  const data = await api(`/api/threads/${state.threadId}`);
  state.messages = data.messages;
  state.usage = data.usage || null;
  state.approvals = data.approvals;
  state.runs = data.runs;
  renderThread();
  renderMeta();
  loadArtifacts();
  loadAttachments();
  subscribe();
}

function subscribe() {
  if (state.stream) state.stream.close();
  if (!state.threadId) return;
  const query = state.token ? `?token=${encodeURIComponent(state.token)}` : "";
  const source = new EventSource(`/api/threads/${state.threadId}/events${query}`);
  state.stream = source;
  source.onmessage = (event) => {
    const payload = JSON.parse(event.data);
    handleEvent(payload);
  };
}

function handleEvent(event) {
  if (event.type === "delta") {
    appendDelta(event.data.text);
    return;
  }
  if (event.type === "message") {
    finishStreaming();
    const message = event.data;
    if (!state.messages.some((item) => item.id === message.id)) state.messages.push(message);
    accumulateUsage(message);
    renderThread();
    return;
  }
  if (event.type === "tool") {
    const message = event.data;
    if (!state.messages.some((item) => item.id === message.id)) state.messages.push(message);
    renderThread();
    return;
  }
  if (event.type === "run") {
    const run = event.data;
    state.runs = [run, ...state.runs.filter((item) => item.id !== run.id)];
    renderMeta();
    return;
  }
  if (event.type === "approval") {
    const approval = event.data;
    state.approvals = [approval, ...state.approvals.filter((item) => item.id !== approval.id)];
    renderThread();
  }
}

function appendDelta(text) {
  if (!state.streaming) {
    const wrap = document.createElement("div");
    wrap.className = "msg-bot";
    const avatar = document.createElement("div");
    avatar.className = "avatar-spark";
    avatar.textContent = "✦";
    const body = document.createElement("div");
    body.className = "msg-body streaming";
    wrap.appendChild(avatar);
    wrap.appendChild(body);
    el("thread").appendChild(wrap);
    state.streaming = body;
  }
  state.streaming.textContent += text;
  const scroller = el("chatScrollContainer");
  scroller.scrollTop = scroller.scrollHeight;
}

function finishStreaming() {
  if (state.streaming) {
    state.streaming.remove();
    state.streaming = null;
  }
}

function renderSpaces() {
  const picker = el("space-select");
  picker.innerHTML = "";
  const all = document.createElement("option");
  all.value = "";
  all.textContent = t("spaces.all");
  picker.appendChild(all);
  for (const space of state.spaces || []) {
    const option = document.createElement("option");
    option.value = space.id;
    option.textContent = space.name;
    picker.appendChild(option);
  }
  picker.value = state.spaceId || "";
}

function renderRoster() {
  const roster = el("roster");
  roster.innerHTML = "";
  const visible = state.bots.filter((bot) => !state.spaceId || (bot.spaceId || "") === state.spaceId);
  el("threadCountBadge").textContent = String(visible.length);
  for (const bot of visible) {
    const item = document.createElement("button");
    item.type = "button";
    if (bot.id === state.botId) item.classList.add("active");
    const name = document.createElement("div");
    name.className = "t-name";
    name.textContent = bot.name;
    const sub = document.createElement("div");
    sub.className = "t-sub";
    sub.textContent = `${bot.threads[0]?.title || "Chat"} · ${spaceName(bot.spaceId)}`;
    item.appendChild(name);
    item.appendChild(sub);
    item.onclick = () => {
      state.botId = bot.id;
      state.threadId = bot.threads[0]?.id || "";
      renderRoster();
      loadThread();
      renderMeta();
      if (typeof loadRoutines === "function") loadRoutines();
    };
    roster.appendChild(item);
  }
  renderMeta();
}

function spaceName(id) {
  if (!id) return "Personal";
  const s = (state.spaces || []).find((x) => x.id === id);
  return s ? s.name : "Space";
}

function accumulateUsage(message) {
  if (!message || (message.promptTokens || 0) <= 0) return;
  const usage = state.usage || { promptTokens: 0, cachedTokens: 0, cachePct: 0 };
  usage.promptTokens += message.promptTokens || 0;
  usage.cachedTokens += message.cachedTokens || 0;
  usage.cachePct = usage.promptTokens > 0 ? (usage.cachedTokens / usage.promptTokens) * 100 : 0;
  state.usage = usage;
  renderMeta();
}

function renderMeta() {
  const bot = currentBot();
  el("bot-name").textContent = bot ? bot.name : "No bot yet";
  let meta = bot ? `${bot.model || "default model"} · ${state.approval} mode` : "";
  if (state.usage && state.usage.promptTokens > 0) {
    const pct = Math.round(state.usage.cachePct || 0);
    meta += ` · ⚡${pct}% cache`;
    el("bot-meta").title = `${state.usage.cachedTokens}/${state.usage.promptTokens} prompt tokens served from cache`;
  } else {
    el("bot-meta").removeAttribute("title");
  }
  const run = activeRun();
  if (run) meta += ` · ${run.status.replace("_", " ")}`;
  el("bot-meta").textContent = meta;
  el("model-pill").textContent = bot ? bot.model || "default model" : "";
  renderToolsBadge();
}

function renderThread() {
  finishStreaming();
  const thread = el("thread");
  thread.innerHTML = "";

  for (const approval of state.approvals.filter((item) => item.status === "pending")) {
    thread.appendChild(approvalCard(approval));
  }

  const rows = feedRows(state.messages);
  for (const row of rows) thread.appendChild(row);

  if (!state.messages.length && !state.approvals.length) {
    const empty = document.createElement("p");
    empty.className = "muted";
    empty.textContent = state.botId
      ? "Tell the bot what to do. It works until it finishes or needs you."
      : "Create a bot to start.";
    thread.appendChild(empty);
  }
  const scroller = el("chatScrollContainer");
  scroller.scrollTop = scroller.scrollHeight;
}

// feedRows folds tool activity into one row per call: the assistant's tool
// calls first, then the matching result row with the error flag attached.
function feedRows(messages) {
  const outputs = new Map();
  for (const message of messages) {
    if (message.role === "tool" && message.toolCallId) {
      outputs.set(message.toolCallId, { output: message.content, error: message.content.startsWith("error:") });
    }
  }
  const rows = [];
  for (const message of messages) {
    if (message.role === "assistant") {
      rows.push(assistantNode(message, outputs));
      continue;
    }
    if (message.role === "tool") {
      continue;
    }
    if (message.role === "user") {
      const wrap = document.createElement("div");
      wrap.className = "msg-user";
      const bubble = document.createElement("div");
      bubble.className = "bubble";
      bubble.textContent = message.content;
      wrap.appendChild(bubble);
      rows.push(wrap);
    }
  }
  return rows;
}

function assistantNode(message, outputs) {
  const wrap = document.createElement("div");
  wrap.className = "msg-bot";
  const avatar = document.createElement("div");
  avatar.className = "avatar-spark";
  avatar.textContent = "✦";
  wrap.appendChild(avatar);
  const body = document.createElement("div");
  body.className = "msg-body";
  if (message.toolCalls) {
    let calls = [];
    try {
      calls = JSON.parse(message.toolCalls);
    } catch (error) {
      calls = [];
    }
    for (const call of calls) {
      const name = call.function?.name || "tool";
      const result = outputs.get(call.id);
      body.appendChild(toolRow(name, summarise(name, call.function?.arguments || ""), !!result?.error, result?.output || ""));
    }
  }
  if (message.content) body.appendChild(renderMarkdown(message.content));
  const arts = artifactsForMessage(message);
  if (arts.length) body.appendChild(artifactChips(arts));
  wrap.appendChild(body);
  return wrap;
}

// artifactsForMessage matches this assistant turn with files it produced:
// the tool result rows that directly follow it in the same turn.
function artifactsForMessage(message) {
  if (!message.toolCalls) return [];
  let calls = [];
  try {
    calls = JSON.parse(message.toolCalls);
  } catch (error) {
    return [];
  }
  const paths = [];
  for (const call of calls) {
    if ((call.function?.name || "") !== "write_file") continue;
    try {
      const args = JSON.parse(call.function?.arguments || "{}");
      if (args.path) paths.push(args.path);
    } catch (error) {
      /* ignore */
    }
  }
  return state.artifactFiles.filter((a) => paths.includes(a.path));
}

function artifactChips(files) {
  const box = document.createElement("div");
  box.className = "artifacts";
  box.style.padding = "0";
  box.style.margin = "4px 0 0";
  for (const art of files) {
    const card = document.createElement("div");
    card.className = "artifact-card";
    const icon = document.createElement("span");
    icon.className = "artifact-icon";
    icon.textContent = "<>";
    const meta = document.createElement("div");
    meta.className = "artifact-meta";
    const name = document.createElement("span");
    name.className = "artifact-name";
    name.textContent = art.path;
    const sub = document.createElement("span");
    sub.className = "artifact-sub";
    sub.textContent = `${art.size || "?"} · Open →`;
    meta.appendChild(name);
    meta.appendChild(sub);
    card.appendChild(icon);
    card.appendChild(meta);
    card.onclick = () => openArtifactFile(art.path);
    box.appendChild(card);
  }
  return box;
}

// renderMarkdown converts the small Markdown subset models emit (headings,
// bold, italic, inline code, fenced blocks, lists, links, quotes) into DOM
// nodes. It never uses innerHTML: every text node is escaped by construction.
function renderMarkdown(source) {
  const root = document.createElement("div");
  root.className = "md";
  const lines = String(source).split("\n");
  let i = 0;
  let list = null;
  const flushList = () => { list = null; };
  while (i < lines.length) {
    const line = lines[i];
    if (/^```/.test(line)) {
      flushList();
      const body = [];
      i++;
      while (i < lines.length && !/^```/.test(lines[i])) { body.push(lines[i]); i++; }
      i++;
      const pre = document.createElement("pre");
      pre.textContent = body.join("\n");
      root.appendChild(pre);
      continue;
    }
    const heading = /^(#{1,4})\s+(.*)$/.exec(line);
    if (heading) {
      flushList();
      const level = Math.min(heading[1].length + 1, 4);
      const tag = document.createElement("h" + level);
      appendInline(tag, heading[2]);
      root.appendChild(tag);
      i++;
      continue;
    }
    const quote = /^>\s?(.*)$/.exec(line);
    if (quote) {
      flushList();
      const block = document.createElement("blockquote");
      appendInline(block, quote[1]);
      root.appendChild(block);
      i++;
      continue;
    }
    const bullet = /^[-*]\s+(.*)$/.exec(line);
    const ordered = /^\d+[.)]\s+(.*)$/.exec(line);
    if (bullet || ordered) {
      const wantOrdered = !!ordered;
      if (!list || list.dataset.ordered !== String(wantOrdered)) {
        list = document.createElement(wantOrdered ? "ol" : "ul");
        list.dataset.ordered = String(wantOrdered);
        root.appendChild(list);
      }
      const item = document.createElement("li");
      appendInline(item, (bullet || ordered)[1]);
      list.appendChild(item);
      i++;
      continue;
    }
    if (/^\s*$/.test(line)) { flushList(); i++; continue; }
    flushList();
    const para = document.createElement("p");
    appendInline(para, line);
    root.appendChild(para);
    i++;
  }
  return root;
}

function appendInline(parent, text) {
  const pattern = /(\*\*.+?\*\*|\*[^*\n]+\*|`[^`\n]+`|\[[^\]]+\]\([^)\s]+\))/g;
  let last = 0;
  let match;
  while ((match = pattern.exec(text)) !== null) {
    if (match.index > last) parent.appendChild(document.createTextNode(text.slice(last, match.index)));
    const token = match[0];
    if (token.startsWith("**")) {
      const strong = document.createElement("strong");
      strong.textContent = token.slice(2, -2);
      parent.appendChild(strong);
    } else if (token.startsWith("`")) {
      const code = document.createElement("code");
      code.textContent = token.slice(1, -1);
      parent.appendChild(code);
    } else if (token.startsWith("[")) {
      const end = token.indexOf("](");
      const label = token.slice(1, end);
      const href = token.slice(end + 2, -1);
      if (/^https?:\/\//.test(href)) {
        const link = document.createElement("a");
        link.textContent = label;
        link.href = href;
        link.target = "_blank";
        link.rel = "noopener noreferrer";
        parent.appendChild(link);
      } else {
        parent.appendChild(document.createTextNode(token));
      }
    } else {
      const em = document.createElement("em");
      em.textContent = token.slice(1, -1);
      parent.appendChild(em);
    }
    last = match.index + token.length;
  }
  if (last < text.length) parent.appendChild(document.createTextNode(text.slice(last)));
}

function summarise(name, rawArgs) {
  let args = {};
  try {
    args = JSON.parse(rawArgs || "{}");
  } catch (error) {
    args = {};
  }
  for (const key of ["command", "path", "query", "url", "fact", "name", "prompt"]) {
    if (typeof args[key] === "string" && args[key].trim()) {
      const value = args[key].length > 80 ? args[key].slice(0, 80) + "..." : args[key];
      return `${name} ${value}`;
    }
  }
  return name;
}

function toolRow(name, summary, failed, output) {
  const block = document.createElement("div");
  block.className = "tool" + (failed ? " failed" : "");
  const head = document.createElement("span");
  head.textContent = "▸ " + summary;
  block.appendChild(head);
  if (output) {
    const body = document.createElement("pre");
    body.textContent = output;
    block.appendChild(body);
  }
  block.onclick = () => block.classList.toggle("open");
  return block;
}

function approvalCard(approval) {
  const card = document.createElement("div");
  card.className = "card";
  const title = document.createElement("h3");
  title.textContent = `Allow ${approval.tool}?`;
  const args = document.createElement("code");
  args.textContent = approval.args;
  const row = document.createElement("div");
  row.className = "row";
  const allow = document.createElement("button");
  allow.textContent = t("approval.allow");
  allow.onclick = () => decide(approval.id, "allow");
  const deny = document.createElement("button");
  deny.className = "ghost";
  deny.textContent = t("approval.deny");
  deny.onclick = () => decide(approval.id, "deny");
  row.append(allow, deny);
  card.append(title, args, row);
  return card;
}

async function decide(id, decision) {
  await api(`/api/approvals/${id}`, { method: "POST", body: JSON.stringify({ decision }) });
}

async function loadMemories() {
  if (!state.botId) return;
  const memories = await api(`/api/bots/${state.botId}/memories`);
  const list = el("memory-list");
  list.innerHTML = "";
  if (!memories.length) {
    const empty = document.createElement("li");
    empty.className = "meta";
    empty.textContent = t("memory.empty");
    list.appendChild(empty);
    return;
  }
  for (const memory of memories) {
    const item = document.createElement("li");
    const text = document.createElement("div");
    text.innerHTML = `<div></div><div class="meta"></div>`;
    text.firstChild.textContent = memory.content;
    text.lastChild.textContent = memory.botId ? t("memory.scope.you") : t("memory.scope.shared");
    const remove = document.createElement("button");
    remove.className = "ghost";
    remove.textContent = t("memory.forget");
    remove.onclick = async () => {
      await api(`/api/memories/${memory.id}`, { method: "DELETE" });
      loadMemories();
    };
    item.append(text, remove);
    list.appendChild(item);
  }
}

async function loadScratchpad() {
  if (!state.botId) return;
  const notes = await api(`/api/bots/${state.botId}/scratchpad`);
  const list = el("scratchpad-list");
  list.innerHTML = "";
  if (!notes.length) {
    const empty = document.createElement("li");
    empty.className = "meta";
    empty.textContent = t("notes.empty");
    list.appendChild(empty);
    return;
  }
  for (const note of notes) {
    const item = document.createElement("li");
    item.textContent = note;
    list.appendChild(item);
  }
}

async function loadSkills() {
  const skills = await api("/api/skills");
  const list = el("skill-list");
  list.innerHTML = "";
  if (!skills.length) {
    const empty = document.createElement("li");
    empty.className = "meta";
    empty.textContent = t("skills.empty");
    list.appendChild(empty);
    return;
  }
  for (const skill of skills) {
    const item = document.createElement("li");
    const text = document.createElement("div");
    text.innerHTML = `<div></div><div class="meta"></div>`;
    text.firstChild.textContent = `/${skill.name}`;
    text.lastChild.textContent = skill.description || "";
    const remove = document.createElement("button");
    remove.className = "ghost";
    remove.textContent = "Delete";
    remove.onclick = async () => {
      await api(`/api/skills/${skill.id}`, { method: "DELETE" });
      loadSkills();
    };
    item.append(text, remove);
    list.appendChild(item);
  }
}

async function loadRules() {
  const rules = await api(state.botId ? `/api/rules?botId=${state.botId}` : "/api/rules");
  const list = el("rule-list");
  list.innerHTML = "";
  if (!rules.length) {
    const empty = document.createElement("li");
    empty.className = "meta";
    empty.textContent = t("rules.empty");
    list.appendChild(empty);
    return;
  }
  for (const rule of rules) {
    const item = document.createElement("li");
    const text = document.createElement("div");
    text.innerHTML = `<div></div><div class="meta"></div>`;
    text.firstChild.textContent = `${rule.kind === "allow" ? "Always allow" : "Never allow"} ${rule.tool || "any tool"}`;
    text.lastChild.textContent = (rule.scope ? "this bot" : "every bot") + (rule.args ? ` · ${rule.args}` : "");
    const remove = document.createElement("button");
    remove.className = "ghost";
    remove.textContent = "Delete";
    remove.onclick = async () => {
      await api(`/api/rules/${rule.id}`, { method: "DELETE" });
      loadRules();
    };
    item.append(text, remove);
    list.appendChild(item);
  }
}

async function loadInstructions() {
  if (!state.botId) return;
  const data = await api(`/api/bots/${state.botId}/instructions`);
  const list = el("instruction-list");
  list.innerHTML = "";
  if (!data.versions.length && !data.instructions) {
    const empty = document.createElement("li");
    empty.className = "meta";
    empty.textContent = t("instructions.empty");
    list.appendChild(empty);
    return;
  }
  const current = document.createElement("li");
  current.innerHTML = `<div></div><div class="meta">active</div>`;
  current.firstChild.textContent = data.instructions || "(empty)";
  list.appendChild(current);
  for (const version of data.versions) {
    if (version.undone) continue;
    const item = document.createElement("li");
    const text = document.createElement("div");
    text.innerHTML = `<div></div><div class="meta"></div>`;
    text.firstChild.textContent = version.instructions;
    text.lastChild.textContent = `${version.source} · ${new Date(version.createdAt * 1000).toLocaleString()}`;
    const undo = document.createElement("button");
    undo.className = "ghost";
    undo.textContent = "Undo";
    undo.onclick = async () => {
      const updated = await api(`/api/instructions/${version.id}/undo`, { method: "POST" });
      const form = el("bot-form");
      form.instructions.value = updated.instructions || "";
      loadInstructions();
      await loadState();
    };
    item.append(text, undo);
    list.appendChild(item);
  }
}

async function dictate() {
  try {
    const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
    const recorder = new MediaRecorder(stream);
    const chunks = [];
    recorder.ondataavailable = (event) => chunks.push(event.data);
    const done = new Promise((resolve) => { recorder.onstop = resolve; });
    recorder.start();
    setTimeout(() => recorder.stop(), 8000);
    await done;
    stream.getTracks().forEach((track) => track.stop());
    const audio = new Blob(chunks, { type: recorder.mimeType || "audio/webm" });
    const form = new FormData();
    form.append("audio", audio, "dictation.webm");
    const result = await api("/api/voice/dictate", { method: "POST", body: form });
    el("input").value = (el("input").value + " " + result.text).trim();
  } catch (error) {
    el("bot-meta").textContent = error.message;
  }
}

async function speakLast() {
  if (!state.threadId) return;
  try {
    const audio = await fetch(`/api/threads/${state.threadId}/speak`).then((r) => {
      if (!r.ok) throw new Error("voice unavailable");
      return r.blob();
    });
    new Audio(URL.createObjectURL(audio)).play();
  } catch (error) {
    el("bot-meta").textContent = error.message;
  }
}

async function send(content) {
  if (!state.threadId || !content.trim()) return;
  const data = await api(`/api/threads/${state.threadId}/messages`, {
    method: "POST",
    body: JSON.stringify({ content }),
  });
  if (!state.messages.some((item) => item.id === data.message.id)) state.messages.push(data.message);
  state.runs = [data.run, ...state.runs];
  renderThread();
  renderMeta();
}

async function loadRoutines() {
  if (!state.botId) return;
  const routines = await api(`/api/bots/${state.botId}/routines`);
  const list = el("routine-list");
  list.innerHTML = "";
  if (!routines.length) {
    const empty = document.createElement("li");
    empty.className = "meta";
    empty.textContent = t("routines.empty");
    list.appendChild(empty);
    return;
  }
  for (const routine of routines) {
    const item = document.createElement("li");
    const text = document.createElement("div");
    text.innerHTML = `<div></div><div class="meta"></div>`;
    text.firstChild.textContent = routine.name;
    const meta = `${routine.cron} · next ${routine.nextAt ? new Date(routine.nextAt * 1000).toLocaleString() : "never"}`;
    text.lastChild.textContent = routine.lastError ? `${meta} · last error: ${routine.lastError}` : meta;
    const edit = document.createElement("button");
    edit.className = "ghost";
    edit.textContent = "Edit";
    edit.onclick = async () => {
      const name = prompt("Routine name", routine.name);
      if (name === null) return;
      const cron = prompt("Cron schedule", routine.cron);
      if (cron === null) return;
      const watch = prompt("Watch path in workspace (blank for none)", routine.watchPath || "");
      if (watch === null) return;
      const updated = await api(`/api/routines/${routine.id}`, {
        method: "PATCH",
        body: JSON.stringify({ name, cron }),
      });
      await api(`/api/routines/${updated.id}/watch`, {
        method: "PUT",
        body: JSON.stringify({ path: watch }),
      });
      loadRoutines();
    };
    const history = document.createElement("button");
    history.className = "ghost";
    history.textContent = "History";
    history.onclick = async () => {
      const runs = await api(`/api/routines/${routine.id}/runs`);
      text.lastChild.textContent = runs.length
        ? `${meta} · ${runs.length} run${runs.length === 1 ? "" : "s"}: ` + runs.map((run) => run.status).join(", ")
        : `${meta} · no runs yet`;
    };
    const remove = document.createElement("button");
    remove.className = "ghost";
    remove.textContent = "Delete";
    remove.onclick = async () => {
      await api(`/api/routines/${routine.id}`, { method: "DELETE" });
      loadRoutines();
    };
    item.append(text, edit, history, remove);
    list.appendChild(item);
  }
}

function bind() {
  aliasPanels();
  el("send").onclick = () => {
    const content = el("input").value;
    el("input").value = "";
    el("input").style.height = "auto";
    send(content).catch((error) => {
      showToast(error.message);
    });
  };

  el("input").addEventListener("keydown", (event) => {
    if (event.key === "Enter" && !event.shiftKey) {
      event.preventDefault();
      el("send").click();
    }
  });
  el("input").addEventListener("input", (event) => {
    event.target.style.height = "auto";
    event.target.style.height = Math.min(event.target.scrollHeight, 160) + "px";
  });

  el("mic").onclick = dictate;
  el("speak").onclick = speakLast;
  el("sidebar-toggle").onclick = toggleSidebar;
  el("expandSidebarBtn").onclick = toggleSidebar;
  el("mobileBackdrop").onclick = toggleSidebar;
  el("new-space").onclick = () => el("new-space-dialog").showModal();
  el("space-select").onchange = async (event) => {
    state.spaceId = event.target.value;
    await loadState();
  };
  el("delete-thread").onclick = async () => {
    if (!state.threadId || !confirm("Delete this chat thread?")) return;
    const botId = state.botId;
    await api(`/api/bots/${botId}/threads/${state.threadId}`, { method: "DELETE" }).catch(() => null);
    state.threadId = "";
    await loadState();
    showToast("Thread deleted");
  };
  const savedTheme = localStorage.getItem("gobot_theme") || "claude";
  document.documentElement.dataset.theme = savedTheme;
  el("theme-toggle").onclick = () => {
    const next = document.documentElement.dataset.theme === "claude" ? "plain" : "claude";
    document.documentElement.dataset.theme = next;
    localStorage.setItem("gobot_theme", next);
  };
  // quick tools open the drawer panels
  const qt = (id, name) => { el(id).onclick = () => openToolDrawer(name); };
  qt("qt-search", "Search");
  qt("qt-memory", "Memory");
  qt("qt-skills", "Skills");
  qt("qt-board", "Board");
  qt("qt-bot", "Bot settings");
  el("moreToolsBtn").onclick = (event) => {
    event.stopPropagation();
    el("toolsPopover").classList.toggle("hidden");
  };
  document.addEventListener("click", (event) => {
    if (!event.target.closest(".popover-wrap")) el("toolsPopover").classList.add("hidden");
  });
  el("modalClose").onclick = closeToolDrawer;
  el("modalDone").onclick = closeToolDrawer;
  el("toolDrawerModal").addEventListener("click", (event) => {
    if (event.target.id === "toolDrawerModal") closeToolDrawer();
  });
  el("bot-settings-open").onclick = () => openToolDrawer("Bot settings");
  el("artifactHeaderToggle").onclick = toggleArtifactPanel;
  el("artifactClose").onclick = closeArtifactPanel;
  el("artifactViewModeBtn").onclick = () => {
    state.previewMode = state.previewMode === "preview" ? "code" : "preview";
    renderArtifactTabs();
    renderArtifactBody();
  };
  el("artifactReload").onclick = () => renderArtifactBody();
  el("artifactOpenTab").onclick = () => {
    if (state.previewPath) window.open(`/api/threads/${state.threadId}/preview?path=${encodeURIComponent(state.previewPath)}`, "_blank", "noopener");
  };
  el("artifactCopy").onclick = async () => {
    try {
      const text = await fetchText(state.previewPath);
      await navigator.clipboard.writeText(text);
      showToast("Copied to clipboard");
    } catch (error) {
      showToast(error.message);
    }
  };
  el("preview-close").onclick = closePreview;
  el("preview-reload").onclick = () => {
    const frame = el("preview-frame");
    const src = frame.getAttribute("src");
    frame.removeAttribute("src");
    if (src) frame.src = src;
  };
  el("preview-open").onclick = () => {
    const frame = el("preview-frame");
    const src = frame.getAttribute("src");
    if (src) window.open(src, "_blank", "noopener");
  };
  el("attach-clear").onclick = () => {
    el("composerAttachmentBar").classList.add("hidden");
  };

  // Deep links: #/routines, #/settings, #/admin, ... open the panel.
  const panels = ["routines", "memory", "skills", "rules", "settings", "search", "audit", "backup", "board", "admin", "plugins", "scratchpad"];
  window.addEventListener("hashchange", () => openPanel(location.hash.replace("#/", "")));
  window.__openPanel = openPanel;
  function openPanel(name) {
    if (!panels.includes(name)) return;
    el(name).classList.remove("hidden");
    if (name === "routines") loadRoutines();
    if (name === "memory") loadMemories();
    if (name === "skills") loadSkills();
    if (name === "rules") loadRules();
    if (name === "settings" && currentBot()) {
      const bot = currentBot();
      el("settings").classList.remove("hidden");
      loadInstructions();
    }
    if (name === "audit") { el("audit").classList.remove("hidden"); loadAudit(); }
    if (name === "admin") { el("admin").classList.remove("hidden"); loadAdmin(); }
    if (name === "search") el("search").classList.remove("hidden");
    if (name === "backup") el("backup").classList.remove("hidden");
    if (name === "board") loadBoard();
    if (name === "plugins") loadPlugins();
    if (name === "scratchpad") loadScratchpad();
  }
  if (location.hash.startsWith("#/")) openPanel(location.hash.replace("#/", ""));

  el("attach").onchange = async (event) => {
    const file = event.target.files[0];
    if (!file || !state.threadId) return;
    try {
      const form = new FormData();
      form.append("file", file, file.name);
      await api(`/api/threads/${state.threadId}/attachments`, { method: "POST", body: form });
      event.target.value = "";
      loadAttachments();
    } catch (error) {
      el("bot-meta").textContent = error.message;
    }
  };

  el("approval").addEventListener("change", async (event) => {
    state.approval = event.target.value;
    await api("/api/settings", {
      method: "PUT",
      body: JSON.stringify({ approval: state.approval }),
    });
    renderMeta();
  });

  if (el("routines-toggle")) el("routines-toggle").onclick = () => {
    el("routines").classList.toggle("hidden");
    if (!el("routines").classList.contains("hidden")) loadRoutines();
  };

  if (el("memory-toggle")) el("memory-toggle").onclick = () => {
    el("memory").classList.toggle("hidden");
    if (!el("memory").classList.contains("hidden")) loadMemories();
  };

  if (el("scratchpad-toggle")) el("scratchpad-toggle").onclick = () => {
    el("scratchpad").classList.toggle("hidden");
    if (!el("scratchpad").classList.contains("hidden")) loadScratchpad();
  };

  el("scratchpad-form").addEventListener("submit", async (event) => {
    event.preventDefault();
    const input = new FormData(event.target).get("text");
    if (!String(input).trim() || !state.botId) return;
    try {
      await api(`/api/bots/${state.botId}/scratchpad`, {
        method: "POST",
        body: JSON.stringify({ text: String(input).trim() }),
      });
      event.target.reset();
      el("scratchpad-error").textContent = "";
      loadScratchpad();
    } catch (error) {
      el("scratchpad-error").textContent = error.message;
    }
  });

  el("scratchpad-clear").onclick = async () => {
    if (!state.botId) return;
    await api(`/api/bots/${state.botId}/scratchpad`, { method: "DELETE" });
    loadScratchpad();
  };

  if (el("skills-toggle")) el("skills-toggle").onclick = () => {
    el("skills").classList.toggle("hidden");
    if (!el("skills").classList.contains("hidden")) loadSkills();
  };

  if (el("rules-toggle")) el("rules-toggle").onclick = () => {
    el("rules").classList.toggle("hidden");
    if (!el("rules").classList.contains("hidden")) loadRules();
  };

  if (el("search-toggle")) el("search-toggle").onclick = () => {
    el("search").classList.toggle("hidden");
  };

  if (el("admin-toggle")) el("admin-toggle").onclick = async () => {
    el("admin").classList.toggle("hidden");
    if (!el("admin").classList.contains("hidden")) loadAdmin();
  };

  el("lang").value = currentLang();
  el("lang").onchange = (event) => {
    setLang(event.target.value);
    loadState();
  };

  if (el("audit-toggle")) el("audit-toggle").onclick = async () => {
    el("audit").classList.toggle("hidden");
    if (el("audit").classList.contains("hidden")) return;
    loadAudit();
  };

  if (el("backup-toggle")) el("backup-toggle").onclick = () => {
    el("backup").classList.toggle("hidden");
  };

  if (el("board-toggle")) el("board-toggle").onclick = () => {
    el("board").classList.toggle("hidden");
    if (!el("board").classList.contains("hidden")) loadBoard();
  };

  el("board-form").addEventListener("submit", async (event) => {
    event.preventDefault();
    const title = new FormData(event.target).get("title");
    if (!String(title).trim() || !state.botId) return;
    await api(`/api/bots/${state.botId}/board`, {
      method: "POST",
      body: JSON.stringify({ title: String(title).trim() }),
    });
    event.target.reset();
    loadBoard();
  });

  if (el("plugins-toggle")) el("plugins-toggle").onclick = () => {
    el("plugins").classList.toggle("hidden");
    if (!el("plugins").classList.contains("hidden")) loadPlugins();
  };

  el("plugin-form").addEventListener("submit", async (event) => {
    event.preventDefault();
    const path = new FormData(event.target).get("path");
    if (!String(path).trim() || !state.botId) return;
    try {
      await api(`/api/bots/${state.botId}/plugins`, {
        method: "POST",
        body: JSON.stringify({ path: String(path).trim() }),
      });
      event.target.reset();
      el("plugin-error").textContent = "";
      loadPlugins();
    } catch (error) {
      el("plugin-error").textContent = error.message;
    }
  });

  el("backup-form").addEventListener("submit", async (event) => {
    event.preventDefault();
    const file = new FormData(event.target).get("file");
    if (!file || file.size === 0) {
      el("backup-status").textContent = t("search.pick");
      return;
    }
    const archive = JSON.parse(await file.text());
    await api("/api/backup", { method: "POST", body: JSON.stringify(archive) });
    el("backup-status").textContent = "Restored. Reloading…";
    await loadState();
    el("backup-status").textContent = "Restored.";
  });

  el("search-form").addEventListener("submit", async (event) => {
    event.preventDefault();
    const query = new FormData(event.target).get("q");
    const list = el("search-list");
    list.innerHTML = "";
    try {
      const matches = await api(`/api/search?q=${encodeURIComponent(query)}`);
      if (!matches.length) {
        const empty = document.createElement("li");
        empty.className = "meta";
        empty.textContent = t("search.empty");
        list.appendChild(empty);
        return;
      }
      for (const match of matches) {
        const item = document.createElement("li");
        item.innerHTML = `<div></div><div class="meta"></div>`;
        item.firstChild.textContent = match.content.slice(0, 200);
        item.lastChild.textContent = `${match.role} · ${match.threadId}`;
        list.appendChild(item);
      }
      el("search-error").textContent = "";
    } catch (error) {
      el("search-error").textContent = error.message;
    }
  });

  el("rule-form").addEventListener("submit", async (event) => {
    event.preventDefault();
    const form = event.target;
    let args = form.args.value.trim();
    if (args) {
      try {
        JSON.parse(args);
      } catch (error) {
        el("rule-error").textContent = "Arguments must be valid JSON.";
        return;
      }
    }
    try {
      await api("/api/rules", {
        method: "POST",
        body: JSON.stringify({
          scope: form.shared.checked ? "" : state.botId,
          kind: form.kind.value,
          tool: form.tool.value.trim(),
          args,
        }),
      });
      el("rule-error").textContent = "";
      form.tool.value = "";
      form.args.value = "";
      loadRules();
    } catch (error) {
      el("rule-error").textContent = error.message;
    }
  });

  el("skill-form").addEventListener("submit", async (event) => {
    event.preventDefault();
    const form = event.target;
    try {
      await api("/api/skills", {
        method: "POST",
        body: JSON.stringify({ name: form.name.value, description: form.description.value, body: form.body.value }),
      });
      el("skill-error").textContent = "";
      form.reset();
      loadSkills();
    } catch (error) {
      el("skill-error").textContent = error.message;
    }
  });

  if (el("settings-toggle")) el("settings-toggle").onclick = async () => {
    const panel = el("settings");
    panel.classList.toggle("hidden");
    const bot = currentBot();
    if (bot && !panel.classList.contains("hidden")) {
      const form = el("bot-form");
      form.name.value = bot.name;
      form.model.value = bot.model;
      form.instructions.value = bot.instructions;
      try {
        const fleet = await api(`/api/bots/${bot.id}/fleet`);
        form.fleet.checked = !!fleet.fleet;
      } catch (error) {
        form.fleet.checked = false;
      }
      loadInstructions();
    }
  };

  el("bot-form").addEventListener("submit", async (event) => {
    event.preventDefault();
    const form = event.target;
    try {
      await api(`/api/bots/${state.botId}`, {
        method: "PATCH",
        body: JSON.stringify({
          name: form.name.value,
          model: form.model.value,
          instructions: form.instructions.value,
        }),
      });
      await api(`/api/bots/${state.botId}/fleet`, {
        method: "PUT",
        body: JSON.stringify({ fleet: form.fleet.checked }),
      });
      el("bot-error").textContent = "";
      await loadState();
    } catch (error) {
      el("bot-error").textContent = error.message;
    }
  });

  el("routine-form").addEventListener("submit", async (event) => {
    event.preventDefault();
    const form = event.target;
    try {
      await api(`/api/bots/${state.botId}/routines`, {
        method: "POST",
        body: JSON.stringify({
          name: form.name.value,
          prompt: form.prompt.value,
          cron: form.cron.value,
          threadId: state.threadId,
        }),
      });
      el("routine-error").textContent = "";
      form.prompt.value = "";
      form.name.value = "";
      loadRoutines();
    } catch (error) {
      el("routine-error").textContent = error.message;
    }
  });


  el("new-space-form").addEventListener("submit", async (event) => {
    if (event.submitter?.value === "cancel") return;
    event.preventDefault();
    try {
      const data = await api("/api/spaces", {
        method: "POST",
        body: JSON.stringify({ name: event.target.name.value }),
      });
      el("new-space-dialog").close();
      event.target.reset();
      state.spaceId = data.id;
      await loadState();
    } catch (error) {
      el("bot-meta").textContent = error.message;
    }
  });

  el("delete-space").onclick = async () => {
    if (!state.spaceId) return;
    if (!confirm("Delete this space? Only empty spaces can go.")) return;
    try {
      await api(`/api/spaces/${state.spaceId}`, { method: "DELETE" });
      state.spaceId = "";
      await loadState();
    } catch (error) {
      el("bot-meta").textContent = error.message;
    }
  };


  el("new-group-form").addEventListener("submit", async (event) => {
    if (event.submitter?.value === "cancel") return;
    event.preventDefault();
    try {
      const want = event.target.members.value.split(",").map((n) => n.trim()).filter(Boolean);
      const ids = state.bots.filter((bot) => want.some((n) => bot.name.toLowerCase() === n.toLowerCase())).map((bot) => bot.id);
      const data = await api("/api/groups", {
        method: "POST",
        body: JSON.stringify({ title: event.target.title.value, spaceId: state.spaceId || undefined, members: ids }),
      });
      el("new-group-dialog").close();
      event.target.reset();
      state.threadId = data.thread.id;
      state.botId = state.bots[0]?.id || "";
      await loadState();
    } catch (error) {
      el("bot-meta").textContent = error.message;
    }
  });


  el("new-bot-form").addEventListener("submit", async (event) => {
    const form = event.target;
    if (event.submitter?.value === "cancel") return;
    event.preventDefault();
    try {
      const data = await api("/api/bots", {
        method: "POST",
        body: JSON.stringify({ name: form.name.value, instructions: form.instructions.value, spaceId: state.spaceId || undefined }),
      });
      el("new-bot-dialog").close();
      form.reset();
      state.botId = data.bot.id;
      state.threadId = data.thread.id;
      await loadState();
    } catch (error) {
      el("bot-meta").textContent = error.message;
    }
  });
}

bind();
loadState().catch((error) => {
  el("thread").innerHTML = `<p class="muted"></p>`;
  el("thread").firstChild.textContent = error.message;
});

async function loadArtifacts() {
  if (!state.threadId) return;
  const artifacts = await api(`/api/threads/${state.threadId}/artifacts`);
  state.artifactFiles = artifacts;
  renderThread();
  const panel = el("artifactPanel");
  if (!panel.classList.contains("hidden") && state.previewPath) {
    renderArtifactBody();
  }
}

function fmtSize(bytes) {
  const n = Number(bytes) || 0;
  if (n < 1024) return `${n} B`;
  if (n < 1048576) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / 1048576).toFixed(1)} MB`;
}

function openPreview(path) {
  openArtifactPanel(path);
}

function closePreview() {
  closeArtifactPanel();
}

// ---------- Claude-style chrome: sidebar, artifact panel, drawer, toast ----------

function toggleSidebar() {
  const sidebar = el("sidebar");
  const backdrop = el("mobileBackdrop");
  if (window.innerWidth < 1024) {
    const hidden = sidebar.classList.toggle("sidebar-hidden-mobile");
    backdrop.classList.toggle("hidden", hidden);
  } else {
    sidebar.classList.toggle("hidden");
    el("expandSidebarBtn").classList.toggle("hidden", !sidebar.classList.contains("hidden"));
  }
}

function showToast(message) {
  el("toastMessage").textContent = message;
  el("toastNotification").classList.remove("hidden");
  clearTimeout(showToast.timer);
  showToast.timer = setTimeout(() => el("toastNotification").classList.add("hidden"), 2200);
}

const TOOL_DRAWER = [
  { name: "Search", desc: "Semantic & file search" },
  { name: "Audit", desc: "Approvals & audit trail" },
  { name: "Admin", desc: "Roles & permissions" },
  { name: "Plugins", desc: "MCP & extensions" },
  { name: "Board", desc: "Kanban tasks" },
  { name: "Backup", desc: "Snapshots" },
  { name: "Routines", desc: "Cron triggers" },
  { name: "Memory", desc: "Facts & recall" },
  { name: "Notes", desc: "Scratchpad" },
  { name: "Skills", desc: "Playbooks" },
  { name: "Rules", desc: "Guardrails" },
  { name: "Bot settings", desc: "Model & runtime" },
];

function renderToolsBadge() {
  el("tools-count").textContent = String(TOOL_DRAWER.length);
  const grid = el("allToolsGrid");
  grid.innerHTML = "";
  for (const tool of TOOL_DRAWER) {
    const b = document.createElement("button");
    b.type = "button";
    const n = document.createElement("div");
    n.className = "tg-name";
    n.textContent = tool.name;
    const d = document.createElement("div");
    d.className = "tg-desc";
    d.textContent = tool.desc;
    b.appendChild(n);
    b.appendChild(d);
    b.onclick = () => {
      el("toolsPopover").classList.add("hidden");
      openToolDrawer(tool.name);
    };
    grid.appendChild(b);
  }
}

function openToolDrawer(name) {
  el("modalTitle").textContent = toolDrawerTitle(name);
  const body = el("modalBody");
  body.innerHTML = "";
  const panel = toolDrawerPanel(name);
  if (panel) {
    body.appendChild(panel);
  } else {
    const p = document.createElement("p");
    p.className = "muted";
    p.textContent = `${name}: panel opens here.`;
    body.appendChild(p);
  }
  el("toolDrawerModal").classList.remove("hidden");
}

function toolDrawerTitle(name) {
  return name === "Bot settings" ? "Bot settings" : `${name} · Tool Inspector`;
}

function toolDrawerPanel(name) {
  const map = {
    Search: "search",
    Audit: "audit",
    Admin: "admin",
    Plugins: "plugin-panel",
    Board: "board",
    Backup: "backup",
    Routines: "routine-panel",
    Memory: "memory",
    Notes: "scratchpad",
    Skills: "skill-panel",
    Rules: "rule-panel",
    "Bot settings": "bot-panel",
  };
  const src = document.getElementById(map[name] || "");
  if (!src) return null;
  const clone = src.cloneNode(true);
  clone.removeAttribute("id");
  clone.classList.remove("hidden");
  return clone;
}

function closeToolDrawer() {
  el("toolDrawerModal").classList.add("hidden");
}

function openArtifactPanel(path) {
  state.previewPath = path;
  state.previewMode = "preview";
  renderArtifactTabs();
  renderArtifactBody();
  el("artifactPanel").classList.remove("hidden");
}

function closeArtifactPanel() {
  el("artifactPanel").classList.add("hidden");
  state.previewPath = "";
}

function toggleArtifactPanel() {
  const panel = el("artifactPanel");
  if (panel.classList.contains("hidden")) {
    const first = state.artifactFiles.find((a) => /\.html?$/i.test(a.path)) || state.artifactFiles[0];
    if (first) openArtifactPanel(first.path);
    else showToast("No artifacts in this thread yet");
  } else {
    closeArtifactPanel();
  }
}

function openArtifactFile(path) {
  openArtifactPanel(path);
}

function renderArtifactTabs() {
  const tabs = el("artifactFileTabs");
  tabs.innerHTML = "";
  for (const art of state.artifactFiles) {
    const b = document.createElement("button");
    b.type = "button";
    b.textContent = art.path.split("/").pop();
    if (art.path === state.previewPath) b.classList.add("on");
    b.onclick = () => {
      state.previewPath = art.path;
      state.previewMode = /\.html?$/i.test(art.path) ? "preview" : "code";
      renderArtifactTabs();
      renderArtifactBody();
    };
    tabs.appendChild(b);
  }
  el("artifactViewModeBtn").textContent = state.previewMode === "preview" ? "Code" : "Preview";
}

async function renderArtifactBody() {
  const body = el("artifactBody");
  body.innerHTML = "";
  el("artifactTitle").textContent = (state.previewPath || "").split("/").pop();
  el("artifactPathFile").textContent = state.previewPath || "";
  el("artifactSubtitle").textContent = artifactSubtitle(state.previewPath);
  if (state.previewMode === "code" || !/\.html?$/i.test(state.previewPath || "")) {
    const pre = document.createElement("pre");
    pre.textContent = "Loading…";
    body.appendChild(pre);
    try {
      const text = await api(`/api/threads/${state.threadId}/preview?path=${encodeURIComponent(state.previewPath)}`, { headers: { Accept: "text/plain" } }).catch(() => null);
      pre.textContent = typeof text === "string" ? text : await fetchText(state.previewPath);
    } catch (error) {
      pre.textContent = `Could not load file: ${error.message}`;
    }
    return;
  }
  const frame = document.createElement("iframe");
  frame.sandbox = "allow-scripts";
  frame.title = "Artifact preview";
  frame.src = `/api/threads/${state.threadId}/preview?path=${encodeURIComponent(state.previewPath)}`;
  body.appendChild(frame);
}

async function fetchText(path) {
  const resp = await fetch(`/api/threads/${state.threadId}/preview?path=${encodeURIComponent(path)}`);
  if (!resp.ok) throw new Error(`request failed (${resp.status})`);
  return resp.text();
}

function artifactSubtitle(path) {
  if (/\.html?$/i.test(path || "")) return "Interactive preview";
  if (/\.css$/i.test(path || "")) return "Stylesheet";
  if (/\.js$/i.test(path || "")) return "Script";
  return "Artifact file";
}

async function loadAttachments() {
  if (!state.threadId) return;
  const box = el("attachments");
  if (!box) return;
  box.innerHTML = "";
  let items = [];
  try {
    items = await api(`/api/threads/${state.threadId}/attachments`);
  } catch (error) {
    return;
  }
  for (const item of items) {
    const link = document.createElement("a");
    link.href = `/api/attachments/${item.id}`;
    link.textContent = `🔒 ${item.filename} (${item.size} bytes)`;
    link.download = "";
    box.appendChild(link);
  }
}

async function loadBoard() {
  if (!state.botId) return;
  const cards = await api(`/api/bots/${state.botId}/board`);
  const cols = el("board-cols");
  cols.innerHTML = "";
  for (const status of ["todo", "doing", "done"]) {
    const col = document.createElement("div");
    col.className = "board-col";
    const head = document.createElement("h4");
    head.textContent = status;
    col.appendChild(head);
    for (const card of cards.filter((c) => c.status === status)) {
      const item = document.createElement("div");
      item.className = "board-card";
      item.innerHTML = `<span></span><div class="row"></div>`;
      item.firstChild.textContent = card.title;
      const row = item.lastChild;
      for (const next of ["todo", "doing", "done"]) {
        if (next === card.status) continue;
        const move = document.createElement("button");
        move.className = "ghost";
        move.textContent = "→ " + next;
        move.onclick = async () => {
          await api(`/api/board/${card.id}`, {
            method: "PATCH",
            body: JSON.stringify({ status: next, position: 0 }),
          });
          loadBoard();
        };
        row.appendChild(move);
      }
      const del = document.createElement("button");
      del.className = "ghost";
      del.textContent = "✕";
      del.onclick = async () => {
        await api(`/api/board/${card.id}`, { method: "DELETE" });
        loadBoard();
      };
      row.appendChild(del);
      col.appendChild(item);
    }
    cols.appendChild(col);
  }
}

async function loadPlugins() {
  if (!state.botId) return;
  const plugins = await api(`/api/bots/${state.botId}/plugins`);
  const list = el("plugin-list");
  list.innerHTML = "";
  if (!plugins.length) {
    const empty = document.createElement("li");
    empty.className = "meta";
    empty.textContent = t("plugins.empty");
    list.appendChild(empty);
    return;
  }
  for (const plugin of plugins) {
    const item = document.createElement("li");
    item.innerHTML = `<div></div><div class="row"></div>`;
    item.firstChild.textContent = `${plugin.name} (${plugin.path})`;
    const row = item.lastChild;
    const toggle = document.createElement("button");
    toggle.className = "ghost";
    toggle.textContent = plugin.enabled ? "Disable" : "Enable";
    toggle.onclick = async () => {
      await api(`/api/plugins/${plugin.id}`, {
        method: "PUT",
        body: JSON.stringify({ enabled: !plugin.enabled }),
      });
      loadPlugins();
    };
    const remove = document.createElement("button");
    remove.className = "ghost";
    remove.textContent = "Delete";
    remove.onclick = async () => {
      await api(`/api/plugins/${plugin.id}`, { method: "DELETE" });
      loadPlugins();
    };
    row.append(toggle, remove);
    list.appendChild(item);
  }
}

async function loadAudit() {
  const rows = await api("/api/audit?limit=50");
  const list = el("audit-list");
  list.innerHTML = "";
  for (const row of rows) {
    const item = document.createElement("li");
    item.innerHTML = `<div></div><div class="meta"></div>`;
    item.firstChild.textContent = `${row.action} ${row.target || ""}`.trim();
    item.lastChild.textContent = `${row.actor} · ${new Date(row.createdAt * 1000).toLocaleString()}`;
    list.appendChild(item);
  }
}

async function loadAdmin() {
  const list = el("admin-list");
  list.innerHTML = "";
  const rows = [];
  try {
    const [spaces, mcp, secrets, xagents] = await Promise.all([
      api("/api/spaces").catch(() => []),
      api("/api/mcp/servers").catch(() => []),
      api("/api/secrets").catch(() => []),
      api("/api/xagents").catch(() => []),
    ]);
    rows.push([t("admin.title") + " · spaces", String(spaces.length || 0)]);
    const mcpUp = mcp.filter ? mcp.filter((s) => !String(s.health).startsWith("down")).length : 0;
    rows.push(["MCP", `${mcpUp}/${mcp.length || 0} up`]);
    rows.push(["Secrets", String(secrets.length || 0)]);
    rows.push(["External agents", String(xagents.length || 0)]);
    if (state.botId) {
      try {
        const wallet = await api(`/api/bots/${state.botId}/wallet`);
        rows.push(["Wallet", `${wallet.balance} credits · ${wallet.receipts.length} receipts`]);
      } catch (error) {
        rows.push(["Wallet", "n/a"]);
      }
    }
  } catch (error) {
    rows.push(["Error", error.message]);
  }
  for (const [name, value] of rows) {
    const item = document.createElement("li");
    item.innerHTML = `<div></div><div class="meta"></div>`;
    item.firstChild.textContent = name;
    item.lastChild.textContent = value;
    list.appendChild(item);
  }
}
