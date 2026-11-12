// g-msg-bot: molecule for an assistant turn (avatar + markdown + tools +
// artifacts). Content arrives pre-rendered as safe nodes; this component only
// provides layout slots, never raw HTML strings.
import { LitElement, html, css } from "../vendor/lit.js";
import "./g-avatar.js";

export class GMsgBot extends LitElement {
  static properties = {
    streaming: { type: Boolean, reflect: true },
  };

  static styles = css`
    :host { display: flex; gap: 14px; align-items: flex-start; }
    .body { flex: 1; min-width: 0; display: grid; gap: 10px; }
    :host([streaming]) .body::after { content: "▍"; color: var(--muted); }
  `;

  constructor() {
    super();
    this.streaming = false;
  }

  render() {
    return html`
      <g-avatar kind="spark"></g-avatar>
      <div class="body">
        <slot name="tools"></slot>
        <slot name="markdown"></slot>
        <slot name="artifacts"></slot>
      </div>
    `;
  }
}

customElements.define("g-msg-bot", GMsgBot);
