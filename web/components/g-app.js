// g-app: root organism assembling the dashboard (sidebar + chat + composer
// + artifact panel + tool drawer + toast). Wires bootstrap state/API/SSE.
import { LitElement, html, css } from "../vendor/lit.js";
import { state, api, loadState, loadThread, subscribe, initTheme, setTheme } from "../bootstrap.js";
import { DRAWER_TOOLS } from "./g-tool-drawer.js";
import "./g-sidebar.js";
import "./g-chat.js";
import "./g-composer.js";
import "./g-artifact-panel.js";
import "./g-tool-drawer.js";
import "./g-avatar.js";
import "./g-badge.js";
import "./g-btn.js";

export class GApp extends LitElement {
  static properties = {
    bots: { type: Array },
    spaces: { type: Array },
    spaceId: { type: String },
    approval: { type: String },
    botId: { type: String },
    threadId: { type: String },
    messages: { type: Array },
    usage: { type: Object },
    streaming: { type: String },
    artifactFiles: { type: Array },
    previewPath: { type: String },
    previewMode: { type: String },
    previewCode: { type: String },
    drawerTool: { type: String },
    drawerOpen: { type: Boolean },
    toast: { type: String },
    version: { type: String },
  };

  static styles = css`
    :host { display: flex; height: 100vh; width: 100vw; overflow: hidden; background: var(--bg); color: var(--ink); }
    g-sidebar { width: 272px; flex-shrink: 0; background: var(--surface); border-right: 1px solid var(--border); }
    .workspace { flex: 1; display: flex; min-width: 0; height: 100%; }
    .chat-col { flex: 1; display: flex; flex-direction: column; min-width: 0; height: 100%; }
    .topbar { height: 56px; padding: 0 16px; border-bottom: 1px solid var(--border); display: flex; align-items: center; justify-content: space-between; gap: 12px; flex-shrink: 0; }
    .title-wrap { display: flex; align-items: baseline; gap: 10px; min-width: 0; }
    .title-wrap h1 { font-family: var(--serif); font-size: 20px; font-weight: 500; margin: 0; }
    .scroll { flex: 1; overflow-y: auto; padding: 24px 16px 180px; }
    g-chat { max-width: 760px; margin: 0 auto; display: block; }
    .composer-float { position: relative; padding: 0 16px 12px; background: linear-gradient(to top, var(--bg) 55%, transparent); }
    g-composer { max-width: 760px; margin: 0 auto; display: block; }
    g-artifact-panel { width: 46%; max-width: 640px; border-left: 1px solid var(--border); }
    .toast { position: fixed; top: 16px; left: 50%; transform: translateX(-50%); z-index: 60; background: var(--ink); color: var(--bg); font-size: 12px; padding: 8px 16px; border-radius: 9999px; }
  `;

  constructor() {
    super();
    this.bots = [];
    this.spaces = [];
    this.spaceId = "";
    this.approval = "ask";
    this.botId = "";
    this.threadId = "";
    this.messages = [];
    this.usage = null;
    this.streaming = "";
    this.artifactFiles = [];
    this.previewPath = "";
    this.previewMode = "preview";
    this.previewCode = "";
    this.drawerTool = "";
    this.drawerOpen = false;
    this.toast = "";
    this.version = "";
  }

  connectedCallback() {
    super.connectedCallback();
    initTheme();
    this.refresh();
  }

  async refresh() {
    try {
      const health = await api("/api/health").catch(() => null);
      if (health && health.version) this.version = health.version;
      await loadState();
      this._sync();
      await this.openThread(this.botId, this.threadId);
    } catch (error) {
      this.say(String(error.message || error));
    }
  }

  _sync() {
    this.bots = [...state.bots];
    this.spaces = [...state.spaces];
    this.spaceId = state.spaceId;
    this.approval = state.approval;
    if (!this.bots.some((b) => b.id === state.botId) && this.bots.length) {
      state.botId = this.bots[0].id;
      state.threadId = (this.bots[0].threads[0] || {}).id || "";
    }
    this.botId = state.botId;
    this.threadId = state.threadId;
  }

  async openThread(botId, threadId) {
    state.botId = botId;
    state.threadId = threadId;
    this.botId = botId;
    this.threadId = threadId;
    await loadThread();
    this.messages = [...state.messages];
    this.usage = state.usage;
    await this.loadArtifacts();
    subscribe(threadId, {
      message: (m) => this._onMessage(m),
      delta: (d) => this._onDelta(d),
      run: () => this._sync(),
      approval: () => this.refresh(),
    });
  }

  _onMessage(m) {
    if (!m || m.threadId !== state.threadId) return;
    if (!this.messages.some((x) => x.id === m.id)) this.messages = [...this.messages, m];
    this.streaming = "";
    loadThread().then(() => {
      this.messages = [...state.messages];
      this.usage = state.usage;
      this.loadArtifacts();
    });
  }

  _onDelta(d) {
    if (d && d.threadId === state.threadId) this.streaming = (this.streaming || "") + (d.text || "");
  }

  async send(content) {
    if (!this.threadId || !String(content || "").trim()) return;
    try {
      const data = await api(`/api/threads/${this.threadId}/messages`, {
        method: "POST",
        body: JSON.stringify({ content }),
      });
      if (data.message) this.messages = [...this.messages, data.message];
    } catch (error) {
      this.say(String(error.message || error));
    }
  }

  async loadArtifacts() {
    if (!this.threadId) {
      this.artifactFiles = [];
      return;
    }
    try {
      this.artifactFiles = await api(`/api/threads/${this.threadId}/artifacts`);
      if (this.previewPath) await this.renderPreviewBody();
    } catch (error) {
      this.artifactFiles = [];
    }
  }

  openPreview(path) {
    this.previewPath = path;
    this.previewMode = /\.html?$/i.test(path || "") ? "preview" : "code";
    this.previewCode = "";
    this.renderPreviewBody();
  }

  async renderPreviewBody() {
    if (!this.previewPath || this.previewMode !== "code") return;
    try {
      const text = await api(`/api/threads/${this.threadId}/preview?path=${encodeURIComponent(this.previewPath)}`, { headers: { Accept: "text/plain" } });
      this.previewCode = typeof text === "string" ? text : await (await fetch(`/api/threads/${this.threadId}/preview?path=${encodeURIComponent(this.previewPath)}`)).text();
    } catch (error) {
      this.previewCode = `Could not load file: ${error.message}`;
    }
  }

  say(message) {
    this.toast = message;
    clearTimeout(this._toastTimer);
    this._toastTimer = setTimeout(() => {
      this.toast = "";
    }, 2600);
  }

  metaText() {
    const bot = this.bots.find((b) => b.id === this.botId);
    let meta = bot ? `${bot.model || "default model"} · ${this.approval} mode` : "";
    if (this.usage && this.usage.promptTokens > 0) meta += ` · ⚡${Math.round(this.usage.cachePct || 0)}% cache`;
    return meta;
  }

  botName() {
    const bot = this.bots.find((b) => b.id === this.botId);
    return bot ? bot.name : "No bot yet";
  }

  render() {
    const bot = this.bots.find((b) => b.id === this.botId);
    return html`
      <g-sidebar
        .bots=${this.bots} .spaces=${this.spaces} .spaceId=${this.spaceId}
        .approval=${this.approval} .activeBotId=${this.botId} .version=${this.version}
        @g-select=${(e) => {
          const b = this.bots.find((x) => x.id === e.detail.botId);
          this.openThread(e.detail.botId, ((b && b.threads[0]) || {}).id || "");
        }}
        @g-action=${(e) => this.say(e.detail.action + ": use the header controls")}
      ></g-sidebar>
      <div class="workspace">
        <div class="chat-col">
          <div class="topbar">
            <div class="title-wrap"><h1>${this.botName()}</h1><g-badge>${this.metaText()}</g-badge></div>
            <div>
              <g-btn variant="ghost" @g-click=${() => setTheme(document.documentElement.dataset.theme === "claude" ? "plain" : "claude")}>◐</g-btn>
              <g-btn variant="accent" @g-click=${() => (this.previewPath ? this.closePanel() : this.openFirstArtifact())}>Artifacts</g-btn>
            </div>
          </div>
          <div class="scroll">
            <g-chat
              .messages=${this.messages} .streaming=${this.streaming}
              .artifacts=${this.artifactFiles}
              @g-preview=${(e) => this.openPreview(e.detail.path)}
            ></g-chat>
          </div>
          <div class="composer-float">
            <g-composer
              .model=${(bot && bot.model) || "default model"}
              @g-send=${(e) => this.send(e.detail.content)}
            ></g-composer>
          </div>
        </div>
        ${this.previewPath
          ? html`<g-artifact-panel
              .files=${this.artifactFiles.map((a) => a.path)} .path=${this.previewPath}
              .mode=${this.previewMode} .code=${this.previewCode}
              .previewBase=${`/api/threads/${this.threadId}/preview?path=`}
              @g-file=${(e) => this.openPreview(e.detail.path)}
              @g-close=${() => this.closePanel()}
              @g-copy=${() => this.copyPreview()}
              @g-open=${(e) => window.open(`/api/threads/${this.threadId}/preview?path=${encodeURIComponent(e.detail.path)}`, "_blank", "noopener")}
              @g-reload=${() => this.renderPreviewBody()}
            ></g-artifact-panel>`
          : ""}
      </div>
      <g-tool-drawer .tool=${this.drawerTool} .open=${this.drawerOpen} @g-close=${() => (this.drawerOpen = false)}>
        ${DRAWER_TOOLS.map((t) => html`<span slot=${t.slot}>${t.desc}</span>`)}
      </g-tool-drawer>
      ${this.toast ? html`<div class="toast">${this.toast}</div>` : ""}
    `;
  }

  openFirstArtifact() {
    const first = (this.artifactFiles.find((a) => /\.html?$/i.test(a.path)) || this.artifactFiles[0] || {}).path;
    if (first) this.openPreview(first);
    else this.say("No artifacts in this thread yet");
  }

  closePanel() {
    this.previewPath = "";
  }

  async copyPreview() {
    try {
      await this.renderPreviewBody();
      await navigator.clipboard.writeText(this.previewCode || "");
      this.say("Copied to clipboard");
    } catch (error) {
      this.say(error.message);
    }
  }
}

customElements.define("g-app", GApp);
