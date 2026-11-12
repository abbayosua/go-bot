// g-tool-row: molecule for one tool call (name/summary/output/error).
import { LitElement, html, css } from "../vendor/lit.js";

export class GToolRow extends LitElement {
  static properties = {
    name: { type: String },
    summary: { type: String },
    output: { type: String },
    failed: { type: Boolean, reflect: true },
  };

  static styles = css`
    :host { display: block; }
    .row {
      display: grid; gap: 2px; font-size: 12.5px; color: var(--muted);
      background: var(--surface); border: 1px solid var(--border);
      border-radius: 10px; padding: 8px 12px;
    }
    :host([failed]) .row { border-color: var(--error); }
    .head { display: flex; gap: 8px; align-items: baseline; }
    .ticon { color: var(--terra); }
    :host([failed]) .ticon { color: var(--error); }
    .tname { font-weight: 600; color: var(--ink); }
    pre { margin: 4px 0 0; white-space: pre-wrap; word-break: break-word; font-size: 12px; max-height: 160px; overflow: auto; }
  `;

  constructor() {
    super();
    this.name = "tool";
    this.summary = "";
    this.output = "";
    this.failed = false;
  }

  render() {
    return html`
      <div class="row">
        <div class="head"><span class="ticon">${this.failed ? "✕" : "⚙"}</span><span class="tname">${this.name}</span><span>${this.summary}</span></div>
        ${this.output ? html`<pre>${this.output.slice(0, 2000)}</pre>` : ""}
      </div>
    `;
  }
}

customElements.define("g-tool-row", GToolRow);
