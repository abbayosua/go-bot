package msg

import (
	"strings"
	"testing"
)

func TestParseUpdateReadsText(t *testing.T) {
	raw := []byte(`{"update_id":7,"message":{"message_id":1,"chat":{"id":99,"type":"private"},"from":{"id":5,"username":"ana"},"text":"hello bot"}}`)
	message, id := ParseUpdate(raw)
	if message == nil || id != 7 {
		t.Fatalf("parsed %+v %d", message, id)
	}
	if message.ChatID != "99" || message.From != "ana" || message.Text != "hello bot" {
		t.Fatalf("parsed %+v", message)
	}
}

func TestParseUpdateIgnoresNonText(t *testing.T) {
	for _, raw := range []string{
		`{"update_id":1}`,
		`{"update_id":2,"message":{"message_id":1,"chat":{"id":1},"text":"  "}}`,
		`not json`,
	} {
		if message, _ := ParseUpdate([]byte(raw)); message != nil {
			t.Fatalf("parsed %q", raw)
		}
	}
}

func TestSendSplitsLongText(t *testing.T) {
	long := strings.Repeat("x", 9000)
	chunks := splitTelegram(long)
	if len(chunks) != 3 {
		t.Fatalf("chunks %d", len(chunks))
	}
	for _, chunk := range chunks {
		if len(chunk) > 4096 {
			t.Fatalf("oversize chunk %d", len(chunk))
		}
	}
}
