const state = {
  token: localStorage.getItem("gobot_token") || "",
  bots: [],
  approval: "ask",
  botId: "",
  threadId: "",
  messages: [],
  approvals: [],
  runs: [],
  stream: null,
  streaming: null,
};

const el = (id) => document.getElementById(id);

async function api(path, options = {}) {
  const headers = { "Content-Type": "application/json", ...(options.headers || {}) };
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
  const data = await api("/api/state");
  state.bots = data.bots;
  state.approval = data.approval;
  el("approval").value = data.approval;
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
  state.approvals = data.approvals;
  state.runs = data.runs;
  renderThread();
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
    const bubble = document.createElement("div");
    bubble.className = "message assistant streaming";
    el("thread").appendChild(bubble);
    state.streaming = bubble;
  }
  state.streaming.textContent += text;
  const thread = el("thread");
  thread.scrollTop = thread.scrollHeight;
}

function finishStreaming() {
  if (state.streaming) {
    state.streaming.remove();
    state.streaming = null;
  }
}

function renderRoster() {
  const roster = el("bots");
  roster.innerHTML = "";
  for (const bot of state.bots) {
    const wrapper = document.createElement("div");
    const item = document.createElement("div");
    item.className = `bot-item${bot.id === state.botId ? " active" : ""}`;
    item.innerHTML = `<div class="name"></div><div class="thread"></div>`;
    item.querySelector(".name").textContent = bot.name;
    item.querySelector(".thread").textContent = bot.threads[0]?.title || "No chat";
    item.onclick = () => {
      state.botId = bot.id;
      state.threadId = bot.threads[0]?.id || "";
      renderRoster();
      loadThread();
      renderMeta();
      loadRoutines();
    };
    wrapper.appendChild(item);
    roster.appendChild(wrapper);
  }
  renderMeta();
}

function renderMeta() {
  const bot = currentBot();
  el("bot-name").textContent = bot ? bot.name : "No bot yet";
  el("bot-meta").textContent = bot
    ? `${bot.model || "default model"} · ${state.approval} mode`
    : "";
  const run = activeRun();
  if (run) {
    el("bot-meta").textContent += ` · ${run.status.replace("_", " ")}`;
  }
}

function renderThread() {
  const thread = el("thread");
  finishStreaming();
  thread.innerHTML = "";

  for (const message of state.messages) {
    if (message.role === "tool") {
      const block = document.createElement("div");
      block.className = "tool";
      block.textContent = `${message.toolName} → ${message.content}`;
      block.onclick = () => block.classList.toggle("open");
      thread.appendChild(block);
      continue;
    }
    if (message.role !== "user" && message.role !== "assistant") continue;
    if (!message.content) continue;
    const bubble = document.createElement("div");
    bubble.className = `message ${message.role}`;
    bubble.textContent = message.content;
    thread.appendChild(bubble);
  }

  for (const approval of state.approvals.filter((item) => item.status === "pending")) {
    thread.appendChild(approvalCard(approval));
  }

  if (!state.messages.length && !state.approvals.length) {
    const empty = document.createElement("p");
    empty.className = "muted";
    empty.textContent = state.botId
      ? "Tell the bot what to do. It works until it finishes or needs you."
      : "Create a bot to start.";
    thread.appendChild(empty);
  }
  thread.scrollTop = thread.scrollHeight;
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
  allow.textContent = "Allow";
  allow.onclick = () => decide(approval.id, "allow");
  const deny = document.createElement("button");
  deny.className = "ghost";
  deny.textContent = "Deny";
  deny.onclick = () => decide(approval.id, "deny");
  row.append(allow, deny);
  card.append(title, args, row);
  return card;
}

async function decide(id, decision) {
  await api(`/api/approvals/${id}`, { method: "POST", body: JSON.stringify({ decision }) });
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
}

async function loadRoutines() {
  if (!state.botId) return;
  const routines = await api(`/api/bots/${state.botId}/routines`);
  const list = el("routine-list");
  list.innerHTML = "";
  if (!routines.length) {
    const empty = document.createElement("li");
    empty.className = "meta";
    empty.textContent = "No routines yet.";
    list.appendChild(empty);
    return;
  }
  for (const routine of routines) {
    const item = document.createElement("li");
    const text = document.createElement("div");
    text.innerHTML = `<div></div><div class="meta"></div>`;
    text.firstChild.textContent = routine.name;
    text.lastChild.textContent = `${routine.cron} · next ${new Date(routine.nextAt * 1000).toLocaleString()}`;
    const remove = document.createElement("button");
    remove.className = "ghost";
    remove.textContent = "Delete";
    remove.onclick = async () => {
      await api(`/api/routines/${routine.id}`, { method: "DELETE" });
      loadRoutines();
    };
    item.append(text, remove);
    list.appendChild(item);
  }
}

function bind() {
  el("composer").addEventListener("submit", async (event) => {
    event.preventDefault();
    const input = el("input");
    const content = input.value;
    input.value = "";
    try {
      await send(content);
    } catch (error) {
      el("bot-meta").textContent = error.message;
    }
  });

  el("input").addEventListener("keydown", (event) => {
    if (event.key === "Enter" && !event.shiftKey) {
      event.preventDefault();
      el("composer").requestSubmit();
    }
  });

  el("approval").addEventListener("change", async (event) => {
    state.approval = event.target.value;
    await api("/api/settings", {
      method: "PUT",
      body: JSON.stringify({ approval: state.approval }),
    });
    renderMeta();
  });

  el("routines-toggle").onclick = () => {
    el("routines").classList.toggle("hidden");
    if (!el("routines").classList.contains("hidden")) loadRoutines();
  };

  el("settings-toggle").onclick = () => {
    const panel = el("settings");
    panel.classList.toggle("hidden");
    const bot = currentBot();
    if (bot && !panel.classList.contains("hidden")) {
      const form = el("bot-form");
      form.name.value = bot.name;
      form.model.value = bot.model;
      form.instructions.value = bot.instructions;
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

  el("new-bot").onclick = () => el("new-bot-dialog").showModal();

  el("new-bot-form").addEventListener("submit", async (event) => {
    const form = event.target;
    if (event.submitter?.value === "cancel") return;
    event.preventDefault();
    try {
      const data = await api("/api/bots", {
        method: "POST",
        body: JSON.stringify({ name: form.name.value, instructions: form.instructions.value }),
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
