// g-sidebar: organism (brand / collapsible controls / thread roster / footer).
// Emits g-select {botId,threadId}, g-space {spaceId}, g-approval {approval},
// g-lang {lang}, g-action {action} for new-chat/new-bot/delete-thread/etc.
import { LitElement, html, css } from "../vendor/lit.js";

export class GSidebar extends LitElement {
  static properties = {
    bots: { type: Array },
    activeBotId: { type: String },
    activeThreadId: { type: String },
    approval: { type: String },
    spaces: { type: Array },
    spaceId: { type: String },
    version: { type: String },
    lang: { type: String },
    collapsed: { type: Object },
  };

  static styles = css`
    :host { display: flex; flex-direction: column; justify-content: space-between; height: 100%; min-height: 0; }
    .head { padding: 14px 14px 10px; display: flex; flex-direction: column; gap: 8px; }
    .brand { display: flex; align-items: center; gap: 8px; min-width: 0; }
    .dot { width: 8px; height: 8px; border-radius: 9999px; background: #16a34a; box-shadow: 0 0 0 4px rgb(22 163 74 / 0.15); flex-shrink: 0; }
    strong { font-family: var(--serif); font-weight: 500; font-size: 19px; letter-spacing: -0.015em; }
    .ver { color: var(--muted); font-size: 11px; }
    .btn-row { display: flex; gap: 6px; }
    .btn-row g-btn { flex: 1; }
    .controls { padding: 0 14px 14px; display: grid; gap: 8px; border-bottom: 1px solid var(--border); }
    .sect { border: 1px solid var(--border); border-radius: 10px; background: var(--surface); overflow: hidden; }
    .sect-head { width: 100%; border: 0; background: none; font: inherit; font-size: 12px; font-weight: 600; color: var(--muted); text-transform: uppercase; letter-spacing: 0.06em; padding: 8px 10px; display: flex; justify-content: space-between; align-items: center; cursor: pointer; }
    .sect-body { padding: 8px 10px 10px; display: grid; gap: 8px; }
    .space-row { display: flex; gap: 6px; align-items: center; }
    .space-row select { flex: 1; font: inherit; border: 1px solid var(--border); border-radius: 8px; padding: 6px 8px; background: var(--bg); color: var(--ink); font-size: 12.5px; }
    .mini { font: inherit; cursor: pointer; border: 1px solid var(--border); border-radius: 8px; padding: 6px 8px; background: var(--surface); color: var(--ink); font-size: 12px; white-space: nowrap; }
    .mini:hover { background: var(--hover); }
    .mini.danger { color: var(--error); }
    .list { flex: 1; overflow-y: auto; padding: 12px 10px; display: flex; flex-direction: column; gap: 4px; }
    .list-head { display: flex; justify-content: space-between; padding: 0 8px 6px; font-size: 11px; text-transform: uppercase; letter-spacing: 0.06em; color: var(--muted); }
    button.item {
      width: 100%; text-align: left; border: 1px solid transparent; background: none; color: var(--ink);
      border-radius: 12px; padding: 10px 12px; cursor: pointer;
    }
    button.item:hover { background: var(--hover); }
    button.item.on { background: var(--surface-strong); border-color: var(--border); }
    .t-name { font-size: 13.5px; font-weight: 600; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
    .t-sub { font-size: 11.5px; color: var(--muted); margin-top: 2px; }
    .field { display: flex; align-items: center; gap: 8px; font-size: 12.5px; color: var(--muted); }
    .field select { flex: 1; font: inherit; border: 1px solid var(--border); border-radius: 8px; padding: 6px 8px; background: var(--bg); color: var(--ink); font-size: 12.5px; }
    .foot {
      margin: 12px; padding: 8px; border: 1px solid var(--border); border-radius: 12px;
      display: flex; align-items: center; gap: 10px; cursor: pointer; background: rgb(255 255 255 / 0.5);
      color: var(--ink); font-size: 13px; width: calc(100% - 24px);
    }
    .avatar { width: 28px; height: 28px; border-radius: 9999px; background: var(--terra); color: #fff; display: flex; align-items: center; justify-content: center; font-size: 12px; font-weight: 600; flex-shrink: 0; }
  `;

  constructor() {
    super();
    this.bots = [];
    this.activeBotId = "";
    this.activeThreadId = "";
    this.approval = "ask";
    this.spaces = [];
    this.spaceId = "";
    this.version = "";
    this.lang = localStorage.getItem("gobot_lang") || "en";
    let collapsed = {};
    try { collapsed = JSON.parse(localStorage.getItem("gobot_collapsed") || "{}"); } catch {}
    this.collapsed = collapsed;
  }

  render() {
    const visible = (this.bots || []).filter((b) => !this.spaceId || (b.spaceId || "") === this.spaceId);
    const spaceName = (id) => ((this.spaces || []).find((s) => s.id === id) || {}).name || "";
    const totalThreads = visible.reduce((n, b) => n + ((b.threads || []).length || 1), 0);
    return html`
      <div class="head">
        <div class="brand"><span class="dot"></span><strong>Go-Bot</strong>${this.version ? html`<span class="ver">${this.version}</span>` : ""}</div>
        <div class="btn-row">
          <g-btn variant="ghost" @g-click=${() => this._act("new-chat")}>+ New chat</g-btn>
          <g-btn variant="ghost" @g-click=${() => this._act("new-bot")}>+ New bot</g-btn>
          <g-btn variant="ghost" @g-click=${() => this._act("delete-thread")}>Delete</g-btn>
        </div>
      </div>
      <div class="controls">
        <div class="sect">
          <button class="sect-head" @click=${() => this._toggle("spaces")}><span>Spaces</span><span>${this.collapsed.spaces ? "+" : "−"}</span></button>
          ${this.collapsed.spaces ? "" : html`<div class="sect-body">
            <div class="space-row">
              <select .value=${this.spaceId} @change=${(e) => this._space(e.target.value)}>
                <option value="">All spaces</option>
                ${(this.spaces || []).map((s) => html`<option value=${s.id}>${s.name || s.id}</option>`)}
              </select>
              <button class="mini" title="New space" @click=${() => this._act("new-space")}>+</button>
              <button class="mini danger" title="Delete space" @click=${() => this._act("delete-space")}>×</button>
              <button class="mini" @click=${() => this._act("new-group")}>Group</button>
            </div>
          </div>`}
        </div>
        <div class="sect">
          <button class="sect-head" @click=${() => this._toggle("approvals")}><span>Approvals</span><span>${this.collapsed.approvals ? "+" : "−"}</span></button>
          ${this.collapsed.approvals ? "" : html`<div class="sect-body">
            <label class="field">Mode
              <select .value=${this.approval} @change=${(e) => this._approval(e.target.value)}>
                <option value="ask">Always ask</option>
                <option value="auto">Auto</option>
                <option value="never">Never ask</option>
              </select>
            </label>
          </div>`}
        </div>
        <div class="sect">
          <button class="sect-head" @click=${() => this._toggle("lang")}><span>Language</span><span>${this.collapsed.lang ? "+" : "−"}</span></button>
          ${this.collapsed.lang ? "" : html`<div class="sect-body">
            <label class="field">Language
              <select .value=${this.lang} @change=${(e) => this._lang(e.target.value)}>
                <option value="en">English</option>
                <option value="id">Indonesia</option>
              </select>
            </label>
          </div>`}
        </div>
        <slot name="controls"></slot>
      </div>
      <div class="list">
        <div class="list-head"><span>Agents & Chats</span><span>${totalThreads}</span></div>
        ${visible.map((b) => html`
          ${(b.threads && b.threads.length ? b.threads : [{ id: "", title: "Chat" }]).map((t) => html`
            <button class="item ${b.id === this.activeBotId && (!t.id || t.id === this.activeThreadId) ? "on" : ""}" @click=${() => this._select(b.id, t.id)}>
              <div class="t-name">${b.name}</div>
              <div class="t-sub">${t.title || "Chat"}${spaceName(b.spaceId) ? ` · ${spaceName(b.spaceId)}` : ""}</div>
            </button>`)}`)}
      </div>
      <button class="foot" @click=${() => this._act("settings")}>
        <span class="avatar">GB</span><span>Go-Bot Local Agent</span><span style="margin-left:auto">···</span>
      </button>
    `;
  }

  _toggle(key) {
    this.collapsed = { ...this.collapsed, [key]: !this.collapsed[key] };
    try { localStorage.setItem("gobot_collapsed", JSON.stringify(this.collapsed)); } catch {}
  }

  _select(botId, threadId) {
    this.dispatchEvent(new CustomEvent("g-select", { detail: { botId, threadId }, bubbles: true, composed: true }));
  }

  _space(spaceId) {
    this.dispatchEvent(new CustomEvent("g-space", { detail: { spaceId }, bubbles: true, composed: true }));
  }

  _approval(approval) {
    this.dispatchEvent(new CustomEvent("g-approval", { detail: { approval }, bubbles: true, composed: true }));
  }

  _lang(lang) {
    this.lang = lang;
    try { localStorage.setItem("gobot_lang", lang); } catch {}
    this.dispatchEvent(new CustomEvent("g-lang", { detail: { lang }, bubbles: true, composed: true }));
  }

  _act(action) {
    this.dispatchEvent(new CustomEvent("g-action", { detail: { action }, bubbles: true, composed: true }));
  }
}

customElements.define("g-sidebar", GSidebar);
