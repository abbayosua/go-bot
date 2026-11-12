// bootstrap.js: state + API + SSE + theme + root component mount.
// No render functions: all DOM lives inside Lit components.
import { apply as applyTheme, loadSaved, save as saveTheme } from "./themes/registry.js";

export const state = {
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
  usage: null,
  stream: null,
  streamingText: "",
};

export async function api(path, options = {}) {
  const headers = { ...(options.headers || {}) };
  if (!(options.body instanceof FormData) && !headers["Content-Type"]) {
    headers["Content-Type"] = "application/json";
  }
  if (state.token) headers.Authorization = `Bearer ${state.token}`;
  const response = await fetch(path, { ...options, headers });
  if (response.status === 401 && !state.token) {
    const provided = prompt("Access token for this Go-Bot instance:");
    if (provided) {
      state.token = provided.trim();
      localStorage.setItem("gobot_token", state.token);
      return api(path, options);
    }
  }
  if (!response.ok) {
    const text = await response.text().catch(() => "");
    throw new Error(text || `request failed (${response.status})`);
  }
  const contentType = response.headers.get("content-type") || "";
  if (contentType.includes("application/json")) return response.json();
  return response.text();
}

export function initTheme() {
  const saved = loadSaved(localStorage);
  applyTheme(document, saved);
  return saved;
}

export function setTheme(id) {
  applyTheme(document, id);
  saveTheme(localStorage, id);
}

export function subscribe(threadId, handlers) {
  if (state.stream) state.stream.close();
  if (!threadId) return;
  const query = state.token ? `?token=${encodeURIComponent(state.token)}` : "";
  const source = new EventSource(`/api/threads/${threadId}/events${query}`);
  state.stream = source;
  source.addEventListener("message", (event) => {
    const payload = JSON.parse(event.data || "{}");
    handleEvent(payload, handlers);
  });
}

function handleEvent(event, handlers = {}) {
  const fn = handlers[event.type];
  if (typeof fn === "function") fn(event.data);
}

export async function loadState() {
  const data = await api(state.spaceId ? `/api/state?spaceId=${state.spaceId}` : "/api/state");
  state.bots = data.bots;
  state.spaces = data.spaces || [];
  state.approval = data.approval;
  return state;
}

export async function loadThread() {
  if (!state.threadId) {
    state.messages = [];
    state.approvals = [];
    state.runs = [];
    return state;
  }
  const data = await api(`/api/threads/${state.threadId}`);
  state.messages = data.messages;
  state.usage = data.usage || null;
  state.approvals = data.approvals;
  state.runs = data.runs;
  return state;
}

export async function boot() {
  initTheme();
  const { GApp } = await import("./components/g-app.js");
  if (!customElements.get("g-app")) {
    void GApp;
  }
  const mount = document.getElementById("app");
  mount.innerHTML = "<g-app></g-app>";
}

boot().catch((error) => {
  document.getElementById("app").innerHTML = `<p style="padding:24px;color:red">${String(error.message || error)}</p>`;
});
