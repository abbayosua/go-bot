// g-composer: organism (textarea + mic/attach/speak + model pill + send).
// Emits g-send {content} on Send click or Enter (Shift+Enter = newline).
import { LitElement, html, css } from "../vendor/lit.js";

export class GComposer extends LitElement {
  static properties = {
    model: { type: String },
    disabled: { type: Boolean },
    attachName: { type: String },
    busy: { type: Boolean },
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
    .icon-btn:disabled { opacity: 0.45; cursor: default; }
    .attach-bar { display: flex; padding: 8px 16px 0; }
    .attach-pill { display: inline-flex; align-items: center; gap: 6px; font-size: 12.5px; color: var(--muted); background: var(--surface); border: 1px solid var(--border); border-radius: 9999px; padding: 4px 6px 4px 10px; }
    .attach-pill button { border: 0; background: none; cursor: pointer; color: var(--muted); font-size: 14px; }
    .hidden-input { display: none; }
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
    this.attachName = "";
    this.busy = false;
  }

  render() {
    return html`
      <div class="card">
        ${this.attachName ? html`<div class="attach-bar"><span class="attach-pill">📄 ${this.attachName} <button @click=${this._clearAttach} title="Remove">×</button></span></div>` : ""}
        <textarea id="box" rows="1" placeholder="Give the bot a direction…" ?disabled=${this.disabled} @keydown=${this._keys} @input=${this._grow}></textarea>
        <input id="file" class="hidden-input" type="file" @change=${this._picked} />
        <div class="bar">
          <div class="row">
            <button class="icon-btn" title="Attach a file" @click=${() => this.shadowRoot.getElementById("file").click()}>📎</button>
            <button class="icon-btn" title="Dictate" ?disabled=${this.busy} @click=${this._dictate}>🎙</button>
            <button class="icon-btn" title="Speak last reply" ?disabled=${this.busy} @click=${this._speak}>🔊</button>
            <slot name="tools"></slot>
          </div>
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

  get box() {
    return this.shadowRoot ? this.shadowRoot.getElementById("box") : null;
  }

  appendText(text) {
    if (!this.box || !text) return;
    this.box.value = (this.box.value + " " + text).trim();
    this.box.dispatchEvent(new Event("input", { bubbles: true }));
    this.box.focus();
  }

  _clearAttach() {
    this.attachName = "";
    const file = this.shadowRoot ? this.shadowRoot.getElementById("file") : null;
    if (file) file.value = "";
    this.dispatchEvent(new CustomEvent("g-attach-clear", { bubbles: true, composed: true }));
  }

  _picked() {
    const file = this.shadowRoot ? this.shadowRoot.getElementById("file") : null;
    const picked = file && file.files && file.files[0];
    if (!picked) return;
    this.attachName = picked.name;
    this.dispatchEvent(new CustomEvent("g-attach", { detail: { file: picked }, bubbles: true, composed: true }));
  }

  async _dictate() {
    try {
      this.busy = true;
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
      const recorder = new MediaRecorder(stream);
      const chunks = [];
      recorder.ondataavailable = (event) => chunks.push(event.data);
      const done = new Promise((resolve) => { recorder.onstop = resolve; });
      recorder.start();
      setTimeout(() => recorder.stop(), 8000);
      await done;
      stream.getTracks().forEach((track) => track.stop());
      const audio = new Blob(chunks, { type: recorder.mimeType || "audio/webm" });
      const form = new FormData();
      form.append("audio", audio, "dictation.webm");
      const res = await fetch("/api/voice/dictate", { method: "POST", body: form });
      if (!res.ok) throw new Error(await res.text());
      const result = await res.json();
      this.appendText(result.text || "");
    } catch (error) {
      this.dispatchEvent(new CustomEvent("g-say", { detail: { message: String(error.message || error) }, bubbles: true, composed: true }));
    } finally {
      this.busy = false;
    }
  }

  async _speak() {
    try {
      this.busy = true;
      const res = await fetch(this.getAttribute("speak-url") || "", { method: "GET" });
      if (!res.ok) throw new Error(res.status === 404 ? "voice unavailable" : await res.text());
      const blob = await res.blob();
      const audio = new Audio(URL.createObjectURL(blob));
      await audio.play();
    } catch (error) {
      this.dispatchEvent(new CustomEvent("g-say", { detail: { message: String(error.message || error) }, bubbles: true, composed: true }));
    } finally {
      this.busy = false;
    }
  }
}

customElements.define("g-composer", GComposer);
