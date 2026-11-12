// g-badge: atomic status pill (model / mode / cache). Second Lit component.
import { LitElement, html, css } from "../vendor/lit.js";

export class GBadge extends LitElement {
  static properties = {
    kind: { type: String, reflect: true },
    tip: { type: String },
  };

  static styles = css`
    :host { display: inline-block; }
    span {
      display: inline-flex; align-items: center; padding: 2px 8px; border-radius: 9999px;
      font-size: 11.5px; font-weight: 500; color: var(--muted, #6E6B64);
      background: var(--surface, #F3F1EC); border: 1px solid var(--border, #E5E2DC);
      white-space: nowrap;
    }
    :host([kind="cache"]) span { color: var(--terra, #D97757); }
    :host([kind="run"]) span { color: var(--ink, #1F1E1D); }
  `;

  constructor() {
    super();
    this.kind = "model";
    this.tip = "";
  }

  render() {
    return html`<span title=${this.tip || ""}><slot></slot></span>`;
  }
}

customElements.define("g-badge", GBadge);
