// g-tool-drawer: organism modal hosting the 12 existing tool panels as
// named slots. Host page projects each panel; component only switches.
import { LitElement, html, css } from "../vendor/lit.js";

export const DRAWER_TOOLS = [
  { name: "Search", desc: "Semantic & file search", slot: "search" },
  { name: "Audit", desc: "Approvals & audit trail", slot: "audit" },
  { name: "Admin", desc: "Roles & permissions", slot: "admin" },
  { name: "Plugins", desc: "MCP & extensions", slot: "plugins" },
  { name: "Board", desc: "Kanban tasks", slot: "board" },
  { name: "Backup", desc: "Snapshots", slot: "backup" },
  { name: "Routines", desc: "Cron triggers", slot: "routines" },
  { name: "Memory", desc: "Facts & recall", slot: "memory" },
  { name: "Notes", desc: "Scratchpad", slot: "notes" },
  { name: "Skills", desc: "Playbooks", slot: "skills" },
  { name: "Rules", desc: "Guardrails", slot: "rules" },
  { name: "Bot settings", desc: "Model & runtime", slot: "settings" },
];

export class GToolDrawer extends LitElement {
  static properties = {
    tool: { type: String },
    open: { type: Boolean, reflect: true },
  };

  static styles = css`
    :host { display: contents; }
    .backdrop {
      position: fixed; inset: 0; z-index: 50; display: flex;
      align-items: center; justify-content: center; padding: 16px;
      background: rgb(0 0 0 / 0.25);
    }
    .card { background: var(--bg); border: 1px solid var(--border); border-radius: 16px; width: 100%; max-width: 560px; overflow: hidden; }
    .head { padding: 16px 20px; background: var(--surface); border-bottom: 1px solid var(--border); display: flex; align-items: center; justify-content: space-between; }
    .head h2 { margin: 0; font-family: var(--serif); font-size: 20px; font-weight: 500; }
    .icon { border: 0; background: none; color: var(--muted); border-radius: 8px; padding: 6px 8px; cursor: pointer; }
    .body { padding: 20px; font-size: 14px; display: grid; gap: 12px; max-height: 65vh; overflow-y: auto; }
    .foot { padding: 12px 20px; background: var(--surface); border-top: 1px solid var(--border); display: flex; justify-content: flex-end; }
    .send { background: var(--terra); color: #fff; border: 0; border-radius: 12px; padding: 6px 16px; font-size: 12px; font-weight: 500; cursor: pointer; }
  `;

  constructor() {
    super();
    this.tool = "";
    this.open = false;
  }

  title() {
    return this.tool === "Bot settings" ? "Bot settings" : `${this.tool} · Tool Inspector`;
  }

  slotFor(name) {
    const found = DRAWER_TOOLS.find((t) => t.name === name);
    return found ? found.slot : "";
  }

  render() {
    if (!this.open) return html``;
    const slot = this.slotFor(this.tool);
    return html`
      <div class="backdrop" @click=${this._backdrop}>
        <div class="card" @click=${(e) => e.stopPropagation()}>
          <div class="head"><h2>${this.title()}</h2><button class="icon" @click=${this.close} title="Close">×</button></div>
          <div class="body"><slot name=${slot}></slot></div>
          <div class="foot"><button class="send" @click=${this.close}>Done</button></div>
        </div>
      </div>
    `;
  }

  _backdrop() {
    this.close();
  }

  close() {
    this.open = false;
    this.dispatchEvent(new CustomEvent("g-close", { bubbles: true, composed: true }));
  }
}

customElements.define("g-tool-drawer", GToolDrawer);
