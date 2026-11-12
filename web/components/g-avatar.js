// g-avatar: atomic avatar (spark for assistant, initials for bots).
// Third Lit atom.
import { LitElement, html, css } from "../vendor/lit.js";

export class GAvatar extends LitElement {
  static properties = {
    kind: { type: String, reflect: true },
    initials: { type: String },
  };

  static styles = css`
    :host { display: inline-flex; flex-shrink: 0; }
    .box {
      width: 28px; height: 28px; border-radius: 8px;
      display: flex; align-items: center; justify-content: center;
      font-size: 15px; font-weight: 600;
    }
    :host([kind="spark"]) .box {
      background: var(--terra-soft, rgb(217 119 87 / 0.12));
      border: 1px solid var(--terra-border, rgb(217 119 87 / 0.25));
      color: var(--terra, #D97757);
    }
    :host([kind="bot"]) .box {
      background: var(--terra, #D97757); color: #fff; font-size: 12px;
    }
  `;

  constructor() {
    super();
    this.kind = "spark";
    this.initials = "GB";
  }

  render() {
    return html`<span class="box">${this.kind === "bot" ? this.initials : "✦"}</span>`;
  }
}

customElements.define("g-avatar", GAvatar);
