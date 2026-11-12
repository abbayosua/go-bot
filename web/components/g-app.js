// g-app: root organism assembling the dashboard (sidebar + chat + composer
// + artifact panel + tool drawer + toast). Wires bootstrap state/API/SSE.
import { LitElement, html, css } from "../vendor/lit.js";
import { state, api, loadState, loadThread, subscribe, initTheme, setTheme } from "../bootstrap.js";
import "./g-sidebar.js";
import "./g-chat.js";
import "./g-composer.js";
import "./g-artifact-panel.js";
import "./g-tool-drawer.js";
import "./g-panels.js";
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
    toolsOpen: { type: Boolean },
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
    .tools { display: flex; align-items: center; gap: 4px; }
    .tool-btn { font: inherit; cursor: pointer; border: 0; background: none; color: var(--muted); font-size: 12.5px; font-weight: 500; padding: 6px 10px; border-radius: 8px; }
    .tool-btn:hover { color: var(--ink); background: var(--hover); }
    .tools-pop { position: relative; }
    .tools-menu { position: absolute; top: calc(100% + 6px); right: 0; z-index: 50; min-width: 220px; background: var(--surface); border: 1px solid var(--border); border-radius: 12px; padding: 6px; box-shadow: 0 8px 24px rgb(0 0 0 / 0.12); display: grid; }
    .tools-menu button { text-align: left; border: 0; background: none; font: inherit; font-size: 12.5px; padding: 8px 10px; border-radius: 8px; cursor: pointer; color: var(--ink); display: flex; justify-content: space-between; gap: 12px; }
    .tools-menu button:hover { background: var(--hover); }
    .tools-menu span { color: var(--muted); font-size: 11.5px; }
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
    this.toolsOpen = false;
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

  async newChat() {
    if (!this.botId) { this.say("No bot yet"); return; }
    try {
      const data = await api(`/api/bots/${this.botId}/threads`, { method: "POST", body: JSON.stringify({}) });
      const thread = data.thread || data;
      await this.refresh();
      if (thread && thread.id) await this.openThread(this.botId, thread.id);
      this.say("New chat started");
    } catch (error) {
      this.say(String(error.message || error));
    }
  }

  async newSpace() {
    const name = prompt("Space name:");
    if (!name || !name.trim()) return;
    try {
      const data = await api("/api/spaces", { method: "POST", body: JSON.stringify({ name: name.trim() }) });
      await this.refresh();
      if (data && data.id) this.spaceId = data.id;
      this.say("Space created");
    } catch (error) {
      this.say(String(error.message || error));
    }
  }

  async newGroup() {
    const name = prompt("Group name:");
    if (!name || !name.trim()) return;
    try {
      await api("/api/groups", { method: "POST", body: JSON.stringify({ name: name.trim(), botIds: this.botId ? [this.botId] : [] }) });
      await this.refresh();
      this.say("Group created");
    } catch (error) {
      this.say(String(error.message || error));
    }
  }

  handleAction(action) {
    if (action === "new-chat") return this.newChat();
    if (action === "new-space") return this.newSpace();
    if (action === "new-group") return this.newGroup();
    if (action === "settings") return this.openTool("Bot settings");
    this.say(`${action}: unknown action`);
  }

  get _panelApi() {
    return api;
  }

  openTool(name) {
    this.drawerTool = name;
    this.drawerOpen = true;
    this.toolsOpen = false;
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
        @g-action=${(e) => this.handleAction(e.detail.action)}
      ></g-sidebar>
      <div class="workspace">
        <div class="chat-col">
          <div class="topbar">
            <div class="title-wrap"><h1>${this.botName()}</h1><g-badge>${this.metaText()}</g-badge></div>
            <div class="tools">
              <button class="tool-btn" @click=${() => this.openTool("Search")}>Search</button>
              <button class="tool-btn" @click=${() => this.openTool("Memory")}>Memory</button>
              <button class="tool-btn" @click=${() => this.openTool("Skills")}>Skills</button>
              <div class="tools-pop">
                <button class="tool-btn" @click=${() => (this.toolsOpen = !this.toolsOpen)}>More Tools ▾</button>
                ${this.toolsOpen
                  ? html`<div class="tools-menu">
                      ${["Search", "Audit", "Admin", "Plugins", "Board", "Backup", "Routines", "Memory", "Notes", "Skills", "Rules", "Bot settings"].map(
                        (t) => html`<button @click=${() => this.openTool(t)}>${t}</button>`,
                      )}
                    </div>`
                  : ""}
              </div>
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
        <g-panel-search slot="search" .botId=${this.botId} .api=${this._panelApi}></g-panel-search>
        <g-panel-audit slot="audit" .botId=${this.botId} .api=${this._panelApi}></g-panel-audit>
        <g-panel-admin slot="admin" .botId=${this.botId} .api=${this._panelApi}></g-panel-admin>
        <g-panel-plugins slot="plugins" .botId=${this.botId} .api=${this._panelApi}></g-panel-plugins>
        <g-panel-board slot="board" .botId=${this.botId} .api=${this._panelApi}></g-panel-board>
        <g-panel-backup slot="backup" .botId=${this.botId} .api=${this._panelApi}></g-panel-backup>
        <g-panel-routines slot="routines" .botId=${this.botId} .api=${this._panelApi}></g-panel-routines>
        <g-panel-memory slot="memory" .botId=${this.botId} .api=${this._panelApi}></g-panel-memory>
        <g-panel-notes slot="notes" .botId=${this.botId} .api=${this._panelApi}></g-panel-notes>
        <g-panel-skills slot="skills" .botId=${this.botId} .api=${this._panelApi}></g-panel-skills>
        <g-panel-rules slot="rules" .botId=${this.botId} .api=${this._panelApi}></g-panel-rules>
        <g-panel-settings slot="settings" .botId=${this.botId} .api=${this._panelApi}></g-panel-settings>
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
