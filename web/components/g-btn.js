// g-btn: atomic button (ghost / primary / icon). First Lit component.
import { LitElement, html, css } from "../vendor/lit.js";

export class GBtn extends LitElement {
  static properties = {
    variant: { type: String, reflect: true },
    disabled: { type: Boolean, reflect: true },
  };

  static styles = css`
    :host { display: inline-block; }
    button {
      font: inherit; cursor: pointer; border-radius: 8px;
      padding: 6px 12px; font-size: 12.5px; font-weight: 500;
      border: 1px solid transparent; background: none; color: var(--ink);
    }
    button:hover:not(:disabled) { background: var(--hover); }
    button:disabled { opacity: 0.45; cursor: default; }
    :host([variant="primary"]) button { background: var(--terra); color: #fff; border-color: transparent; }
    :host([variant="primary"]) button:hover:not(:disabled) { background: var(--terra-hover); }
    :host([variant="ghost"]) button { border-color: var(--border); background: var(--surface); }
    :host([variant="icon"]) button { padding: 6px 8px; color: var(--muted); }
    :host([variant="icon"]) button:hover:not(:disabled) { color: var(--ink); }
  `;

  constructor() {
    super();
    this.variant = "ghost";
    this.disabled = false;
  }

  render() {
    return html`<button ?disabled=${this.disabled} @click=${this._forward}><slot></slot></button>`;
  }

  _forward() {
    this.dispatchEvent(new CustomEvent("g-click", { bubbles: true, composed: true }));
  }
}

customElements.define("g-btn", GBtn);
