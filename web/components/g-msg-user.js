// g-msg-user: molecule for the right-aligned user bubble.
// Lit escapes interpolations by construction; content is rendered as text.
import { LitElement, html, css } from "../vendor/lit.js";

export class GMsgUser extends LitElement {
  static properties = {
    content: { type: String },
  };

  static styles = css`
    :host { display: flex; justify-content: flex-end; }
    .bubble {
      max-width: 82%; background: var(--user-bubble);
      border: 1px solid var(--border); border-radius: 16px;
      border-bottom-right-radius: 4px; padding: 12px 16px;
      font-size: 15px; line-height: 1.6;
      white-space: pre-wrap; word-break: break-word;
    }
  `;

  constructor() {
    super();
    this.content = "";
  }

  render() {
    return html`<div class="bubble">${this.content}</div>`;
  }
}

customElements.define("g-msg-user", GMsgUser);
