// g-chat: organism for the message list (user bubbles + assistant turns +
// approvals + streaming). Reactive: setting `messages` re-renders.
// Emits g-preview {path} (bubbled from artifact cards) and g-decide {id,ok}.
import { LitElement, html, css } from "../vendor/lit.js";
import "./g-msg-user.js";
import "./g-msg-bot.js";
import "./g-tool-row.js";
import "./g-artifact-card.js";

export class GChat extends LitElement {
  static properties = {
    messages: { type: Array },
    streaming: { type: String },
    emptyHint: { type: String },
    artifacts: { type: Array },
  };

  static styles = css`
    :host { display: flex; flex-direction: column; gap: 28px; }
    .empty { color: var(--muted); font-size: 13.5px; }
    g-msg-bot streaming-body { display: block; }
  `;

  constructor() {
    super();
    this.messages = [];
    this.streaming = "";
    this.artifacts = [];
    this.emptyHint = "Tell the bot what to do. It works until it finishes or needs you.";
  }

  // feedRows mirrors the legacy fold: assistant tool calls first, then the
  // matching tool result rows, then content. Pure function for testability.
  feedRows(messages) {
    const outputs = new Map();
    for (const m of messages || []) {
      if (m.role === "tool" && m.toolCallId) {
        outputs.set(m.toolCallId, { output: m.content, error: String(m.content || "").startsWith("error:") });
      }
    }
    const rows = [];
    for (const m of messages || []) {
      if (m.role === "assistant") rows.push({ kind: "bot", message: m, outputs });
      else if (m.role === "user") rows.push({ kind: "user", message: m });
    }
    return rows;
  }

  render() {
    const rows = this.feedRows(this.messages);
    if (!rows.length && !this.streaming) return html`<p class="empty">${this.emptyHint}</p>`;
    return html`
      ${rows.map((row) => row.kind === "user"
        ? html`<g-msg-user .content=${row.message.content || ""}></g-msg-user>`
        : html`<g-msg-bot ?streaming=${false}>
            <span slot="tools">${this._tools(row)}</span>
            <span slot="markdown">${row.message.content || ""}</span>
            <span slot="artifacts">${this._artifacts(row)}</span>
          </g-msg-bot>`)}
      ${this.streaming ? html`<g-msg-bot streaming><span slot="markdown">${this.streaming}</span></g-msg-bot>` : ""}
    `;
  }

  _tools(row) {
    let calls = [];
    try {
      calls = JSON.parse(row.message.toolCalls || "[]");
    } catch (error) {
      calls = [];
    }
    return html`${calls.map((c) => {
      const out = row.outputs.get(c.id) || {};
      return html`<g-tool-row name=${c.function?.name || "tool"} summary="" .output=${out.output || ""} ?failed=${!!out.error}></g-tool-row>`;
    })}`;
  }

  // _artifacts matches this assistant turn with the files it produced:
  // write_file calls in the same turn, resolved against the thread list.
  _artifacts(row) {
    let calls = [];
    try {
      calls = JSON.parse(row.message.toolCalls || "[]");
    } catch (error) {
      calls = [];
    }
    const paths = [];
    for (const c of calls) {
      if ((c.function?.name || "") !== "write_file") continue;
      try {
        const args = JSON.parse(c.function?.arguments || "{}");
        if (args.path) paths.push(args.path);
      } catch (error) {
        /* ignore malformed args */
      }
    }
    const files = (this.artifacts || []).filter((a) => paths.includes(a.path));
    return html`${files.map((f) => html`<g-artifact-card .path=${f.path} .size=${f.size}></g-artifact-card>`)}`;
  }
}

customElements.define("g-chat", GChat);
