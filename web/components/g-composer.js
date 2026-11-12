// g-composer: organism (textarea + mic/attach/speak + model pill + send).
// Emits g-send {content} on Send click or Enter (Shift+Enter = newline).
import { LitElement, html, css } from "../vendor/lit.js";

export class GComposer extends LitElement {
  static properties = {
    model: { type: String },
    disabled: { type: Boolean },
  };

  static styles = css`
    :host { display: block; }
    .card {
      background: #fff; border: 1px solid var(--border); border-radius: 22px;
      box-shadow: 0 4px 20px -2px rgb(31 30 29 / 0.06), 0 1px 4px -1px rgb(31 30 29 / 0.04);
    }
    .card:focus-within { border-color: var(--border-strong); }
    textarea {
      width: 100%; box-sizing: border-box; resize: none; border: 0; background: none;
      padding: 14px 16px 8px; font: inherit; font-size: 15px; line-height: 1.6;
      max-height: 160px; color: var(--ink);
    }
    textarea:focus { outline: none; }
    .bar { display: flex; align-items: center; justify-content: space-between; padding: 4px 10px 10px; }
    .row { display: flex; align-items: center; gap: 4px; }
    .icon-btn { border: 0; background: none; color: var(--muted); border-radius: 12px; padding: 8px; cursor: pointer; font-size: 17px; }
    .icon-btn:hover { color: var(--ink); background: var(--surface); }
    .model { font-size: 12.5px; color: var(--muted); padding: 6px 10px; border-radius: 12px; }
    .send {
      width: 36px; height: 36px; border-radius: 12px; background: var(--terra); color: #fff;
      border: 0; cursor: pointer; font-size: 17px; display: flex; align-items: center; justify-content: center;
    }
    .send:hover:not(:disabled) { background: var(--terra-hover); }
    .send:disabled { opacity: 0.45; cursor: default; }
  `;

  constructor() {
    super();
    this.model = "";
    this.disabled = false;
  }

  render() {
    return html`
      <div class="card">
        <textarea id="box" rows="1" placeholder="Give the bot a direction…" ?disabled=${this.disabled} @keydown=${this._keys} @input=${this._grow}></textarea>
        <div class="bar">
          <div class="row"><slot name="tools"></slot></div>
          <div class="row">
            ${this.model ? html`<span class="model">${this.model}</span>` : ""}
            <button class="send" ?disabled=${this.disabled} @click=${this._send} title="Send message">↑</button>
          </div>
        </div>
      </div>
    `;
  }

  _keys(event) {
    if (event.key === "Enter" && !event.shiftKey) {
      event.preventDefault();
      this._send();
    }
  }

  _grow(event) {
    const box = event.target;
    box.style.height = "auto";
    box.style.height = Math.min(box.scrollHeight, 160) + "px";
  }

  _send() {
    const box = this.shadowRoot ? this.shadowRoot.getElementById("box") : null;
    const content = (box ? box.value : "").trim();
    if (!content || this.disabled) return;
    if (box) {
      box.value = "";
      box.style.height = "auto";
    }
    this.dispatchEvent(new CustomEvent("g-send", { detail: { content }, bubbles: true, composed: true }));
  }
}

customElements.define("g-composer", GComposer);
