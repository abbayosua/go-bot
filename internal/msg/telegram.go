package msg

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Telegram speaks the Bot API: sendMessage for outbound, getUpdates for
// inbound polling. Webhooks hit the same update shape through Handler.
type Telegram struct {
	token  string
	base   string
	client *http.Client
}

// NewTelegram builds the adapter. Empty base means api.telegram.org.
func NewTelegram(token, base string) *Telegram {
	if strings.TrimSpace(base) == "" {
		base = "https://api.telegram.org"
	}
	return &Telegram{
		token:  token,
		base:   strings.TrimSuffix(base, "/"),
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (t *Telegram) Name() string { return "telegram" }

func (t *Telegram) endpoint(method string) string {
	return t.base + "/bot" + t.token + "/" + method
}

// Send delivers one message, splitting past the 4096-char limit.
func (t *Telegram) Send(ctx context.Context, chat, text string) error {
	for _, chunk := range splitTelegram(text) {
		form := url.Values{"chat_id": {chat}, "text": {chunk}}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.endpoint("sendMessage"), strings.NewReader(form.Encode()))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		resp, err := t.client.Do(req)
		if err != nil {
			return err
		}
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		resp.Body.Close()
		if resp.StatusCode >= 300 {
			return fmt.Errorf("telegram send status %d: %.200s", resp.StatusCode, raw)
		}
		var ok struct {
			OK          bool   `json:"ok"`
			Description string `json:"description"`
		}
		if err := json.Unmarshal(raw, &ok); err != nil || !ok.OK {
			return fmt.Errorf("telegram send failed: %.200s", raw)
		}
	}
	return nil
}

func splitTelegram(text string) []string {
	if len(text) <= 4096 {
		return []string{text}
	}
	var out []string
	for len(text) > 4096 {
		cut := 4096
		if i := strings.LastIndex(text[:cut], "\n"); i > 3000 {
			cut = i
		}
		out = append(out, text[:cut])
		text = text[cut:]
	}
	return append(out, text)
}

// Update is the subset of getUpdates/webhook payload we read.
type Update struct {
	UpdateID int64 `json:"update_id"`
	Message  *struct {
		MessageID int64 `json:"message_id"`
		Chat      struct {
			ID   int64  `json:"id"`
			Type string `json:"type"`
		} `json:"chat"`
		From *struct {
			ID       int64  `json:"id"`
			Username string `json:"username"`
		} `json:"from"`
		Text string `json:"text"`
	} `json:"message"`
}

// ParseUpdate decodes one webhook body into a Message, or nil for non-text.
func ParseUpdate(body []byte) (*Message, int64) {
	var update Update
	if err := json.Unmarshal(body, &update); err != nil || update.Message == nil {
		return nil, 0
	}
	text := strings.TrimSpace(update.Message.Text)
	if text == "" {
		return nil, update.UpdateID
	}
	from := ""
	if update.Message.From != nil {
		from = update.Message.From.Username
		if from == "" {
			from = fmt.Sprintf("tg:%d", update.Message.From.ID)
		}
	}
	return &Message{
		ChatID: fmt.Sprintf("%d", update.Message.Chat.ID),
		From:   from,
		Text:   text,
	}, update.UpdateID
}

// Poller long-polls getUpdates and ingests each text message.
type Poller struct {
	adapter *Telegram
	ingest  Ingest
	botID   string
	offset  int64
}

// NewPoller builds a poller bound to one bot's install.
func NewPoller(adapter *Telegram, botID string, ingest Ingest) *Poller {
	return &Poller{adapter: adapter, ingest: ingest, botID: botID}
}

// PollOnce fetches one batch and ingests it. The loop lives in the caller.
func (p *Poller) PollOnce(ctx context.Context) error {
	form := url.Values{"timeout": {"25"}, "allowed_updates": {`["message"]`}}
	if p.offset > 0 {
		form.Set("offset", fmt.Sprintf("%d", p.offset))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.adapter.endpoint("getUpdates"), strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := p.adapter.client.Do(req)
	if err != nil {
		return err
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("telegram updates status %d", resp.StatusCode)
	}
	var batch struct {
		OK     bool     `json:"ok"`
		Result []Update `json:"result"`
	}
	if err := json.Unmarshal(raw, &batch); err != nil || !batch.OK {
		return fmt.Errorf("telegram updates decode failed")
	}
	for _, update := range batch.Result {
		p.offset = update.UpdateID + 1
		if update.Message == nil {
			continue
		}
		raw, _ := json.Marshal(update)
		if message, _ := ParseUpdate(raw); message != nil {
			if err := p.ingest(ctx, p.botID, message.ChatID, message.From, message.Text); err != nil {
				return err
			}
		}
	}
	return nil
}
