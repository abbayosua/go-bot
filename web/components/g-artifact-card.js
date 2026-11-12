// g-artifact-card: molecule for one produced file (icon/name/size/actions).
// Clicking an HTML card emits g-preview; download stays a plain link.
import { LitElement, html, css } from "../vendor/lit.js";

const ICONS = { html: "<>", css: "~", js: "{}", json: "{;}", md: "#", txt: "=" };

export class GArtifactCard extends LitElement {
  static properties = {
    path: { type: String },
    size: { type: Number },
  };

  static styles = css`
    :host { display: block; }
    .card {
      display: flex; align-items: center; gap: 12px; background: #fff;
      border: 1px solid var(--border); border-radius: 12px; padding: 10px 14px;
      cursor: pointer;
    }
    .card:hover { border-color: var(--terra-border); }
    .icon {
      width: 32px; height: 32px; border-radius: 8px; flex-shrink: 0;
      background: var(--terra-soft); color: var(--terra);
      display: flex; align-items: center; justify-content: center;
      font-size: 13px; font-weight: 700; font-family: ui-monospace, monospace;
    }
    .meta { display: grid; gap: 1px; min-width: 0; flex: 1; }
    .name { font-size: 13px; font-weight: 600; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
    .sub { font-size: 11px; color: var(--muted); }
    .open { font-size: 10.5px; color: var(--terra); opacity: 0; }
    .card:hover .open { opacity: 1; }
  `;

  constructor() {
    super();
    this.path = "";
    this.size = 0;
  }

  get ext() {
    return (this.path.split(".").pop() || "").toLowerCase();
  }

  get isHtml() {
    return this.ext === "html" || this.ext === "htm";
  }

  get icon() {
    return ICONS[this.ext] || (this.isHtml ? "<>" : "[]");
  }

  fmtSize() {
    const n = Number(this.size) || 0;
    if (n < 1024) return `${n} B`;
    if (n < 1048576) return `${(n / 1024).toFixed(1)} KB`;
    return `${(n / 1048576).toFixed(1)} MB`;
  }

  render() {
    return html`
      <div class="card" @click=${this._preview}>
        <span class="icon">${this.icon}</span>
        <span class="meta">
          <span class="name">${this.path}</span>
          <span class="sub">${this.fmtSize()}${this.isHtml ? " · Interactive preview" : ""}</span>
        </span>
        <span class="open">Open →</span>
      </div>
    `;
  }

  _preview() {
    if (!this.isHtml || !this.path) return;
    this.dispatchEvent(new CustomEvent("g-preview", { detail: { path: this.path }, bubbles: true, composed: true }));
  }
}

customElements.define("g-artifact-card", GArtifactCard);
