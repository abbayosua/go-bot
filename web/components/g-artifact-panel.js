// g-artifact-panel: organism (file tabs / preview-code toggle / sandboxed
// iframe / code view / reload-open-copy). Props drive the view; host page
// supplies previewUrl(path) and loadText(path) callbacks via properties.
import { LitElement, html, css } from "../vendor/lit.js";

export class GArtifactPanel extends LitElement {
  static properties = {
    files: { type: Array },
    path: { type: String },
    mode: { type: String },
    code: { type: String },
  };

  static styles = css`
    :host { display: flex; flex-direction: column; height: 100%; background: #fff; }
    .head { height: 56px; padding: 0 16px; border-bottom: 1px solid var(--border); background: var(--surface); display: flex; align-items: center; justify-content: space-between; gap: 8px; flex-shrink: 0; }
    .title { display: flex; align-items: center; gap: 8px; min-width: 0; }
    .logo { width: 28px; height: 28px; border-radius: 8px; background: var(--terra-soft); color: var(--terra); display: flex; align-items: center; justify-content: center; font-size: 12px; font-weight: 700; flex-shrink: 0; }
    .title h3 { margin: 0; font-size: 13.5px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
    .sub { margin: 0; font-size: 11px; color: var(--muted); }
    .tabs { display: flex; background: var(--surface); border: 1px solid var(--border); border-radius: 8px; padding: 2px; gap: 2px; }
    .tabs button { border: 0; background: none; font-size: 12px; font-weight: 500; color: var(--muted); border-radius: 6px; padding: 4px 10px; cursor: pointer; }
    .tabs button.on { background: #fff; color: var(--ink); box-shadow: 0 1px 2px rgb(0 0 0 / 0.08); }
    .row { display: flex; align-items: center; gap: 8px; }
    .ghost { border: 1px solid var(--border); background: #fff; color: var(--ink); border-radius: 8px; padding: 6px 12px; font-size: 12px; cursor: pointer; }
    .icon { border: 0; background: none; color: var(--muted); border-radius: 8px; padding: 6px 8px; cursor: pointer; }
    .body { flex: 1; overflow: auto; background: #F8F9FA; min-height: 0; }
    .body iframe { width: 100%; height: 100%; border: 0; background: #fff; }
    .body pre { margin: 0; padding: 16px; font-size: 12.5px; font-family: ui-monospace, Menlo, monospace; white-space: pre-wrap; word-break: break-word; }
    .foot { padding: 10px 16px; border-top: 1px solid var(--border); background: var(--surface); display: flex; align-items: center; justify-content: space-between; font-size: 12px; }
    .mono { font-family: ui-monospace, monospace; font-size: 11px; color: var(--muted); }
  `;

  constructor() {
    super();
    this.files = [];
    this.path = "";
    this.mode = "preview";
    this.code = "";
  }

  get isHtml() {
    return /\.html?$/i.test(this.path || "");
  }

  get fileName() {
    return (this.path || "").split("/").pop();
  }

  select(path) {
    this.path = path;
    this.mode = /\.html?$/i.test(path || "") ? "preview" : "code";
    this.code = "";
    this.dispatchEvent(new CustomEvent("g-file", { detail: { path }, bubbles: true, composed: true }));
  }

  toggleMode() {
    this.mode = this.mode === "preview" ? "code" : "preview";
  }

  render() {
    return html`
      <div class="head">
        <div class="title"><span class="logo">&lt;&gt;</span>
          <div><h3>${this.fileName}</h3><p class="sub">${this.isHtml ? "Interactive preview" : "Artifact file"}</p></div>
        </div>
        <div class="row">
          <div class="tabs">
            ${(this.files || []).map((f) => html`<button class=${f === this.path ? "on" : ""} @click=${() => this.select(f)}>${f.split("/").pop()}</button>`)}
          </div>
          <button class="ghost" @click=${this.toggleMode}>${this.mode === "preview" ? "Code" : "Preview"}</button>
          <button class="icon" @click=${() => this.dispatchEvent(new CustomEvent("g-close", { bubbles: true, composed: true }))} title="Close">×</button>
        </div>
      </div>
      <div class="body">
        ${this.mode === "code" || !this.isHtml
          ? html`<pre>${this.code || "Loading…"}</pre>`
          : html`<iframe sandbox="allow-scripts" title="Artifact preview" src=${this.previewSrc}></iframe>`}
      </div>
      <div class="foot">
        <span class="mono">${this.path}</span>
        <span class="row">
          <button class="ghost" @click=${() => this.dispatchEvent(new CustomEvent("g-copy", { detail: { path: this.path }, bubbles: true, composed: true }))}>Copy</button>
          <button class="ghost" @click=${() => this.dispatchEvent(new CustomEvent("g-open", { detail: { path: this.path }, bubbles: true, composed: true }))}>Open in tab</button>
          <button class="ghost" @click=${() => this.dispatchEvent(new CustomEvent("g-reload", { bubbles: true, composed: true }))}>Reload</button>
        </span>
      </div>
    `;
  }

  get previewSrc() {
    return this.previewBase ? this.previewBase + encodeURIComponent(this.path || "") : "";
  }
}

customElements.define("g-artifact-panel", GArtifactPanel);
