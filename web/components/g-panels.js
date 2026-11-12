// g-panels: 12 tool drawer panels as Lit elements, each wired to a real API.
// One component per drawer slot in g-tool-drawer DRAWER_TOOLS.
// All data calls go through the injected `api` function from bootstrap.js.
import { LitElement, html, css } from "../vendor/lit.js";

const panelStyles = css`
  :host { display: block; }
  .stack { display: grid; gap: 10px; }
  .row { display: flex; gap: 8px; align-items: center; flex-wrap: wrap; }
  input, select, textarea {
    font: inherit; border: 1px solid var(--border); border-radius: 8px;
    padding: 7px 10px; background: var(--bg); color: var(--ink); width: 100%;
    box-sizing: border-box;
  }
  textarea { min-height: 64px; resize: vertical; }
  .list { list-style: none; margin: 0; padding: 0; display: grid; gap: 8px; }
  .list li {
    border: 1px solid var(--border); border-radius: 10px; padding: 8px 12px;
    background: var(--surface); display: grid; gap: 4px;
  }
  .meta { color: var(--muted); font-size: 12px; }
  .error { color: var(--error); font-size: 12.5px; min-height: 1em; }
  .empty { color: var(--muted); font-size: 13px; }
  button {
    font: inherit; cursor: pointer; border-radius: 8px; padding: 6px 12px;
    font-size: 12.5px; font-weight: 500; border: 1px solid var(--border);
    background: var(--surface); color: var(--ink);
  }
  button:hover:not(:disabled) { background: var(--hover); }
  button:disabled { opacity: 0.45; cursor: default; }
  button.primary { background: var(--terra); color: #fff; border-color: transparent; }
  button.danger { color: var(--error); border-color: var(--error); }
  .cols { display: grid; grid-template-columns: repeat(3, 1fr); gap: 8px; }
  .col { background: var(--bg); border: 1px solid var(--border); border-radius: 10px; padding: 8px; display: grid; gap: 8px; align-content: start; }
  .col h4 { margin: 0; font-size: 12px; text-transform: uppercase; color: var(--muted); }
  .card { background: var(--surface); border: 1px solid var(--border); border-radius: 8px; padding: 6px 8px; display: grid; gap: 6px; font-size: 13px; }
`;

class PanelBase extends LitElement {
  static properties = {
    botId: { type: String },
    error: { type: String },
    loading: { type: Boolean },
  };
  static styles = panelStyles;

  constructor() {
    super();
    this.botId = "";
    this.error = "";
    this.loading = false;
    this._api = null;
  }

  set api(fn) { this._api = fn; this._onApi(); }
  get api() { return this._api; }

  _onApi() {}

  async call(path, options) {
    if (!this._api) { this.error = "API unavailable"; return null; }
    this.loading = true; this.error = "";
    try {
      return await this._api(path, options);
    } catch (err) {
      this.error = err && err.message ? err.message : String(err);
      return null;
    } finally {
      this.loading = false;
    }
  }

  updated(changed) {
    if (changed.has("botId")) this._onApi();
  }
}

// 1. Search — form + results from GET /api/search
export class GPanelSearch extends PanelBase {
  static properties = { ...super.properties, q: { type: String }, results: { type: Array } };
  constructor() { super(); this.q = ""; this.results = []; }
  render() {
    return html`<div class="stack">
      <form class="row" @submit=${this._search}>
        <input placeholder="Search messages…" .value=${this.q} @input=${(e) => (this.q = e.target.value)} />
        <button class="primary" type="submit" ?disabled=${this.loading}>${this.loading ? "…" : "Search"}</button>
      </form>
      ${this.error ? html`<div class="error">${this.error}</div>` : ""}
      <ul class="list">
        ${this.results.length ? this.results.map((m) => html`<li><div>${m.role || ""} · ${m.threadId || ""}</div><div class="meta">${(m.content || "").slice(0, 220)}</div></li>`) : html`<li class="empty">No results yet.</li>`}
      </ul>
    </div>`;
  }
  async _search(e) {
    e.preventDefault();
    const out = await this.call(`/api/search?q=${encodeURIComponent(this.q || "")}`);
    if (out) this.results = Array.isArray(out) ? out : [];
  }
}
customElements.define("g-panel-search", GPanelSearch);

// 2. Audit — trail + CSV export from GET /api/audit
export class GPanelAudit extends PanelBase {
  static properties = { ...super.properties, items: { type: Array } };
  constructor() { super(); this.items = []; }
  _onApi() { this._load(); }
  async _load() {
    const out = await this.call("/api/audit?limit=100");
    if (out) this.items = Array.isArray(out) ? out : [];
  }
  render() {
    return html`<div class="stack">
      <div class="row"><a href="/api/audit?format=csv&limit=1000">Export CSV</a><span class="meta">${this.items.length} entries</span></div>
      ${this.error ? html`<div class="error">${this.error}</div>` : ""}
      <ul class="list">${this.items.length ? this.items.map((a) => html`<li><div>${a.action || ""}</div><div class="meta">${a.actor || ""} · ${a.target || ""} · ${a.createdAt ? new Date(a.createdAt * 1000).toLocaleString() : ""}</div></li>`) : html`<li class="empty">No audit entries.</li>`}</ul>
    </div>`;
  }
}
customElements.define("g-panel-audit", GPanelAudit);

// 3. Admin — spaces + users roles
export class GPanelAdmin extends PanelBase {
  static properties = { ...super.properties, spaces: { type: Array }, users: { type: Array }, usersError: { type: String }, spaceName: { type: String } };
  constructor() { super(); this.spaces = []; this.users = []; this.usersError = ""; this.spaceName = ""; }
  _onApi() { this._load(); }
  async _load() {
    const sp = await this.call("/api/spaces");
    if (sp) this.spaces = Array.isArray(sp) ? sp : [];
    try {
      const u = this._api ? await this._api("/api/users") : null;
      if (u) this.users = Array.isArray(u) ? u : [];
    } catch (err) {
      this.usersError = err && err.message ? err.message : String(err);
    }
  }
  render() {
    return html`<div class="stack">
      <form class="row" @submit=${this._addSpace}>
        <input placeholder="New space name" .value=${this.spaceName} @input=${(e) => (this.spaceName = e.target.value)} />
        <button class="primary" type="submit">Add space</button>
      </form>
      ${this.error ? html`<div class="error">${this.error}</div>` : ""}
      <div class="meta">Spaces (${this.spaces.length})</div>
      <ul class="list">${this.spaces.map((s) => html`<li><div>${s.name || s.id}</div><div class="meta">${s.id || ""}</div><div class="row"><button class="danger" @click=${() => this._delSpace(s)}>Delete</button></div></li>`)}</ul>
      <div class="meta">Users ${this.usersError ? "· " + this.usersError : `(${this.users.length})`}</div>
      <ul class="list">${this.users.length ? this.users.map((u) => html`<li><div>${u.name || u.email || u.id}</div><div class="meta">${u.role || ""}</div></li>`) : html`<li class="empty">No users visible.</li>`}</ul>
    </div>`;
  }
  async _addSpace(e) {
    e.preventDefault();
    if (!this.spaceName.trim()) return;
    const out = await this.call("/api/spaces", { method: "POST", body: JSON.stringify({ name: this.spaceName.trim() }) });
    if (out !== null) { this.spaceName = ""; this._load(); }
  }
  async _delSpace(s) {
    await this.call(`/api/spaces/${s.id}`, { method: "DELETE" });
    this._load();
  }
}
customElements.define("g-panel-admin", GPanelAdmin);

// 4. Plugins — list/install/enable via bot plugins + MCP servers
export class GPanelPlugins extends PanelBase {
  static properties = { ...super.properties, plugins: { type: Array }, servers: { type: Array }, name: { type: String } };
  constructor() { super(); this.plugins = []; this.servers = []; this.name = ""; }
  _onApi() { this._load(); }
  async _load() {
    if (!this.botId) return;
    const p = await this.call(`/api/bots/${this.botId}/plugins`);
    if (p) this.plugins = Array.isArray(p) ? p : [];
    const m = await this.call("/api/mcp/servers");
    if (m) this.servers = Array.isArray(m) ? m : [];
  }
  render() {
    return html`<div class="stack">
      <form class="row" @submit=${this._install}>
        <input placeholder="plugin folder path" .value=${this.name} @input=${(e) => (this.name = e.target.value)} />
        <button class="primary" type="submit">Install</button>
      </form>
      ${this.error ? html`<div class="error">${this.error}</div>` : ""}
      <div class="meta">Plugins (${this.plugins.length}) · MCP (${this.servers.length})</div>
      <ul class="list">
        ${this.plugins.map((p) => html`<li><div>${p.name || p.id}</div><div class="meta">${p.enabled ? "enabled" : "disabled"} · ${p.id || ""}</div><div class="row"><button @click=${() => this._toggle(p)}>${p.enabled ? "Disable" : "Enable"}</button><button class="danger" @click=${() => this._remove(p)}>Delete</button></div></li>`)}
        ${this.servers.map((s) => html`<li><div>${s.name || s.id}</div><div class="meta">mcp · ${s.id || ""}</div></li>`)}
        ${(!this.plugins.length && !this.servers.length) ? html`<li class="empty">No plugins installed.</li>` : ""}
      </ul>
    </div>`;
  }
  async _install(e) {
    e.preventDefault();
    if (!this.botId || !this.name.trim()) return;
    const out = await this.call(`/api/bots/${this.botId}/plugins`, { method: "POST", body: JSON.stringify({ path: this.name.trim() }) });
    if (out !== null) { this.name = ""; this._load(); }
  }
  async _toggle(p) {
    await this.call(`/api/plugins/${p.id}`, { method: "PUT", body: JSON.stringify({ enabled: !p.enabled }) });
    this._load();
  }
  async _remove(p) {
    await this.call(`/api/plugins/${p.id}`, { method: "DELETE" });
    this._load();
  }
}
customElements.define("g-panel-plugins", GPanelPlugins);

// 5. Board — kanban todo/doing/done via GET+POST /api/bots/{id}/board
export class GPanelBoard extends PanelBase {
  static properties = { ...super.properties, cards: { type: Array }, title: { type: String } };
  constructor() { super(); this.cards = []; this.title = ""; }
  _onApi() { this._load(); }
  async _load() {
    if (!this.botId) return;
    const out = await this.call(`/api/bots/${this.botId}/board`);
    if (out) this.cards = Array.isArray(out) ? out : [];
  }
  render() {
    const cols = ["todo", "doing", "done"];
    return html`<div class="stack">
      <form class="row" @submit=${this._add}>
        <input placeholder="New card title" .value=${this.title} @input=${(e) => (this.title = e.target.value)} />
        <button class="primary" type="submit">Add</button>
      </form>
      ${this.error ? html`<div class="error">${this.error}</div>` : ""}
      <div class="cols">${cols.map((s) => html`<div class="col"><h4>${s}</h4>${this.cards.filter((c) => c.status === s).map((c) => html`<div class="card"><span>${c.title}</span><div class="row">${cols.filter((n) => n !== c.status).map((n) => html`<button @click=${() => this._move(c, n)}>→ ${n}</button>`)}</div><div class="row"><button class="danger" @click=${() => this._del(c)}>Delete</button></div></div>`)}</div>`)}</div>
    </div>`;
  }
  async _add(e) {
    e.preventDefault();
    if (!this.botId || !this.title.trim()) return;
    const out = await this.call(`/api/bots/${this.botId}/board`, { method: "POST", body: JSON.stringify({ title: this.title.trim() }) });
    if (out !== null) { this.title = ""; this._load(); }
  }
  async _move(card, status) {
    await this.call(`/api/board/${card.id}`, { method: "PATCH", body: JSON.stringify({ status }) });
    this._load();
  }
  async _del(card) {
    await this.call(`/api/board/${card.id}`, { method: "DELETE" });
    this._load();
  }
}
customElements.define("g-panel-board", GPanelBoard);

// 6. Backup — snapshot download + restore via /api/backup
export class GPanelBackup extends PanelBase {
  static properties = { ...super.properties, info: { type: String }, restored: { type: String } };
  constructor() { super(); this.info = ""; this.restored = ""; }
  render() {
    return html`<div class="stack">
      <div class="row"><a href="/api/backup">Download backup</a><span class="meta">${this.info}</span></div>
      <div class="row"><input type="file" accept="application/json" @change=${this._restore} /><span class="meta">${this.restored}</span></div>
      ${this.error ? html`<div class="error">${this.error}</div>` : ""}
      <div class="meta">Snapshot is a JSON export of all bots, threads, and settings. Restoring replaces current data.</div>
    </div>`;
  }
  async _onApi() {
    try {
      const res = await fetch("/api/backup", { method: "GET" });
      if (!res.ok) { this.info = ""; return; }
      const blob = await res.blob();
      this.info = `${Math.round(blob.size / 1024)} KB snapshot ready`;
    } catch { this.info = ""; }
  }
  async _restore(e) {
    const file = e.target.files && e.target.files[0];
    if (!file) return;
    const text = await file.text();
    const out = await this.call("/api/backup", { method: "POST", body: text });
    if (out !== null) this.restored = "Restored OK";
  }
}
customElements.define("g-panel-backup", GPanelBackup);

// 7. Routines — CRUD cron per bot
export class GPanelRoutines extends PanelBase {
  static properties = { ...super.properties, routines: { type: Array }, cron: { type: String }, prompt: { type: String } };
  constructor() { super(); this.routines = []; this.cron = ""; this.prompt = ""; }
  _onApi() { this._load(); }
  async _load() {
    if (!this.botId) return;
    const out = await this.call(`/api/bots/${this.botId}/routines`);
    if (out) this.routines = Array.isArray(out) ? out : [];
  }
  render() {
    return html`<div class="stack">
      <form class="stack" @submit=${this._add}>
        <input placeholder="cron (e.g. 0 9 * * *)" .value=${this.cron} @input=${(e) => (this.cron = e.target.value)} />
        <input placeholder="prompt" .value=${this.prompt} @input=${(e) => (this.prompt = e.target.value)} />
        <div class="row"><button class="primary" type="submit">Add routine</button></div>
      </form>
      ${this.error ? html`<div class="error">${this.error}</div>` : ""}
      <ul class="list">${this.routines.length ? this.routines.map((r) => html`<li><div>${r.prompt || r.cron || r.id}</div><div class="meta">${r.cron || ""} · ${r.enabled === false ? "disabled" : "enabled"}</div><div class="row"><button @click=${() => this._toggle(r)}>${r.enabled === false ? "Enable" : "Disable"}</button><button class="danger" @click=${() => this._del(r)}>Delete</button></div></li>`) : html`<li class="empty">No routines.</li>`}</ul>
    </div>`;
  }
  async _add(e) {
    e.preventDefault();
    if (!this.botId || !this.cron.trim() || !this.prompt.trim()) { this.error = "cron and prompt are required"; return; }
    const out = await this.call(`/api/bots/${this.botId}/routines`, { method: "POST", body: JSON.stringify({ cron: this.cron.trim(), prompt: this.prompt.trim() }) });
    if (out !== null) { this.cron = ""; this.prompt = ""; this._load(); }
  }
  async _toggle(r) {
    await this.call(`/api/routines/${r.id}`, { method: "PATCH", body: JSON.stringify({ enabled: r.enabled === false }) });
    this._load();
  }
  async _del(r) {
    await this.call(`/api/routines/${r.id}`, { method: "DELETE" });
    this._load();
  }
}
customElements.define("g-panel-routines", GPanelRoutines);

// 8. Memory — facts list/delete
export class GPanelMemory extends PanelBase {
  static properties = { ...super.properties, items: { type: Array } };
  constructor() { super(); this.items = []; }
  _onApi() { this._load(); }
  async _load() {
    if (!this.botId) return;
    const out = await this.call(`/api/bots/${this.botId}/memories`);
    if (out) this.items = Array.isArray(out) ? out : [];
  }
  render() {
    return html`<div class="stack">
      <div class="row"><button @click=${this._load}>Refresh</button><span class="meta">${this.items.length} facts</span></div>
      ${this.error ? html`<div class="error">${this.error}</div>` : ""}
      <ul class="list">${this.items.length ? this.items.map((m) => html`<li><div>${m.content}</div><div class="meta">${m.botId ? "you" : "shared"}</div><div class="row"><button class="danger" @click=${() => this._forget(m)}>Forget</button></div></li>`) : html`<li class="empty">No facts yet — chat "remember …" to store one.</li>`}</ul>
    </div>`;
  }
  async _forget(m) {
    await this.call(`/api/memories/${m.id}`, { method: "DELETE" });
    this._load();
  }
}
customElements.define("g-panel-memory", GPanelMemory);

// 9. Notes — scratchpad add/clear
export class GPanelNotes extends PanelBase {
  static properties = { ...super.properties, notes: { type: Array }, text: { type: String } };
  constructor() { super(); this.notes = []; this.text = ""; }
  _onApi() { this._load(); }
  async _load() {
    if (!this.botId) return;
    const out = await this.call(`/api/bots/${this.botId}/scratchpad`);
    if (out) this.notes = Array.isArray(out) ? out : [];
  }
  render() {
    return html`<div class="stack">
      <form class="row" @submit=${this._add}>
        <input placeholder="New note" .value=${this.text} @input=${(e) => (this.text = e.target.value)} />
        <button class="primary" type="submit">Add</button>
        <button type="button" @click=${this._clear}>Clear</button>
      </form>
      ${this.error ? html`<div class="error">${this.error}</div>` : ""}
      <ul class="list">${this.notes.length ? this.notes.map((n) => html`<li><div>${typeof n === "string" ? n : (n.text || n.content || "")}</div></li>`) : html`<li class="empty">No notes.</li>`}</ul>
    </div>`;
  }
  async _add(e) {
    e.preventDefault();
    if (!this.botId || !this.text.trim()) return;
    const out = await this.call(`/api/bots/${this.botId}/scratchpad`, { method: "POST", body: JSON.stringify({ text: this.text.trim() }) });
    if (out !== null) { this.text = ""; this._load(); }
  }
  async _clear() {
    if (!this.botId) return;
    await this.call(`/api/bots/${this.botId}/scratchpad`, { method: "DELETE" });
    this._load();
  }
}
customElements.define("g-panel-notes", GPanelNotes);

// 10. Skills — playbook list/add/delete
export class GPanelSkills extends PanelBase {
  static properties = { ...super.properties, skills: { type: Array }, name: { type: String }, body: { type: String } };
  constructor() { super(); this.skills = []; this.name = ""; this.body = ""; }
  _onApi() { this._load(); }
  async _load() {
    const out = await this.call("/api/skills");
    if (out) this.skills = Array.isArray(out) ? out : [];
  }
  render() {
    return html`<div class="stack">
      <form class="stack" @submit=${this._add}>
        <input placeholder="skill name" .value=${this.name} @input=${(e) => (this.name = e.target.value)} />
        <textarea placeholder="instructions" .value=${this.body} @input=${(e) => (this.body = e.target.value)}></textarea>
        <div class="row"><button class="primary" type="submit">Add skill</button></div>
      </form>
      ${this.error ? html`<div class="error">${this.error}</div>` : ""}
      <ul class="list">${this.skills.length ? this.skills.map((s) => html`<li><div>${s.name || s.id}</div><div class="meta">${(s.body || s.content || "").slice(0, 160)}</div><div class="row"><button class="danger" @click=${() => this._del(s)}>Delete</button></div></li>`) : html`<li class="empty">No skills.</li>`}</ul>
    </div>`;
  }
  async _add(e) {
    e.preventDefault();
    if (!this.name.trim()) { this.error = "name is required"; return; }
    const out = await this.call("/api/skills", { method: "POST", body: JSON.stringify({ name: this.name.trim(), body: this.body }) });
    if (out !== null) { this.name = ""; this.body = ""; this._load(); }
  }
  async _del(s) {
    await this.call(`/api/skills/${s.id}`, { method: "DELETE" });
    this._load();
  }
}
customElements.define("g-panel-skills", GPanelSkills);

// 11. Rules — guardrails CRUD global/bot
export class GPanelRules extends PanelBase {
  static properties = { ...super.properties, rules: { type: Array }, kind: { type: String }, tool: { type: String }, args: { type: String } };
  constructor() { super(); this.rules = []; this.kind = "allow"; this.tool = ""; this.args = ""; }
  _onApi() { this._load(); }
  async _load() {
    const q = this.botId ? `?botId=${encodeURIComponent(this.botId)}` : "";
    const out = await this.call(`/api/rules${q}`);
    if (out) this.rules = Array.isArray(out) ? out : [];
  }
  render() {
    return html`<div class="stack">
      <form class="stack" @submit=${this._add}>
        <div class="row">
          <select .value=${this.kind} @change=${(e) => (this.kind = e.target.value)}>
            <option value="allow">allow</option>
            <option value="deny">deny</option>
          </select>
          <input placeholder="tool (empty = all)" .value=${this.tool} @input=${(e) => (this.tool = e.target.value)} />
        </div>
        <input placeholder="args match (empty = all)" .value=${this.args} @input=${(e) => (this.args = e.target.value)} />
        <div class="row"><button class="primary" type="submit">Add rule</button><span class="meta">${this.botId ? "scope: bot" : "scope: global"}</span></div>
      </form>
      ${this.error ? html`<div class="error">${this.error}</div>` : ""}
      <ul class="list">${this.rules.length ? this.rules.map((r) => html`<li><div>${r.kind} · ${r.tool || "all tools"} · ${r.args || "all args"}</div><div class="meta">${r.scope || "global"} · ${r.id || ""}</div><div class="row"><button class="danger" @click=${() => this._del(r)}>Delete</button></div></li>`) : html`<li class="empty">No rules.</li>`}</ul>
    </div>`;
  }
  async _add(e) {
    e.preventDefault();
    const payload = { scope: this.botId || "", kind: this.kind, tool: this.tool.trim(), args: this.args.trim() };
    const out = await this.call("/api/rules", { method: "POST", body: JSON.stringify(payload) });
    if (out !== null) { this.tool = ""; this.args = ""; this._load(); }
  }
  async _del(r) {
    await this.call(`/api/rules/${r.id}`, { method: "DELETE" });
    this._load();
  }
}
customElements.define("g-panel-rules", GPanelRules);

// 12. Bot settings — name/model/instructions/spaces/fleet + undo history
export class GPanelSettings extends PanelBase {
  static properties = {
    ...super.properties, name: { type: String }, model: { type: String },
    instructions: { type: String }, versions: { type: Array },
    wallet: { type: Object }, fleet: { type: Boolean },
  };
  constructor() { super(); this.name = ""; this.model = ""; this.instructions = ""; this.versions = []; this.wallet = null; this.fleet = false; }
  _onApi() { this._load(); }
  async _load() {
    if (!this.botId) return;
    const [ins, wallet, fleet] = await Promise.all([
      this.call(`/api/bots/${this.botId}/instructions`),
      this.call(`/api/bots/${this.botId}/wallet`),
      this.call(`/api/bots/${this.botId}/fleet`),
    ]);
    if (ins) { this.instructions = ins.instructions || ""; this.versions = ins.versions || []; }
    if (wallet) this.wallet = wallet;
    if (fleet) this.fleet = !!fleet.fleet;
  }
  render() {
    return html`<div class="stack">
      <form class="stack" @submit=${this._save}>
        <input placeholder="bot name" .value=${this.name} @input=${(e) => (this.name = e.target.value)} />
        <input placeholder="model (empty = default)" .value=${this.model} @input=${(e) => (this.model = e.target.value)} />
        <textarea placeholder="instructions" .value=${this.instructions} @input=${(e) => (this.instructions = e.target.value)}></textarea>
        <div class="row"><button class="primary" type="submit">Save</button>
          <button type="button" @click=${this._fleet}>Fleet: ${this.fleet ? "on" : "off"}</button></div>
      </form>
      ${this.error ? html`<div class="error">${this.error}</div>` : ""}
      <div class="meta">Wallet: ${this.wallet ? (this.wallet.balance ?? 0) : "—"}</div>
      <div class="meta">Instruction history (${this.versions.length})</div>
      <ul class="list">${this.versions.length ? this.versions.map((v) => html`<li><div>${(v.instructions || "").slice(0, 160)}</div><div class="meta">${v.source || ""} · ${v.createdAt ? new Date(v.createdAt * 1000).toLocaleString() : ""}</div>${v.undone ? "" : html`<div class="row"><button @click=${() => this._undo(v)}>Undo to here</button></div>`}</li>`) : html`<li class="empty">No history.</li>`}</ul>
    </div>`;
  }
  async _save(e) {
    e.preventDefault();
    if (!this.botId) return;
    const body = {};
    if (this.name.trim()) body.name = this.name.trim();
    if (this.model.trim()) body.model = this.model.trim();
    if (this.instructions.trim()) body.instructions = this.instructions.trim();
    if (!Object.keys(body).length) { this.error = "nothing to save"; return; }
    const out = await this.call(`/api/bots/${this.botId}`, { method: "PATCH", body: JSON.stringify(body) });
    if (out !== null) this._load();
  }
  async _fleet() {
    if (!this.botId) return;
    await this.call(`/api/bots/${this.botId}/fleet`, { method: "PUT", body: JSON.stringify({ fleet: !this.fleet }) });
    this._load();
  }
  async _undo(v) {
    await this.call(`/api/instructions/${v.id}/undo`, { method: "POST" });
    this._load();
  }
}
customElements.define("g-panel-settings", GPanelSettings);
