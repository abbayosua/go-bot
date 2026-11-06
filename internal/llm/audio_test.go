package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAudioFailOpen(t *testing.T) {
	ctx := context.Background()
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer plain.Close()
	client := New(plain.URL, "")
	if _, err := client.Transcribe(ctx, "m", "a.wav", []byte("x")); err == nil {
		t.Fatal("transcribe without audio support accepted")
	}
	if _, _, err := client.Speak(ctx, "m", "alloy", "hi"); err == nil {
		t.Fatal("speak without audio support accepted")
	}
	// A provider that speaks returns bytes and a type.
	rich := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/audio/transcriptions" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"text":"hello there"}`))
			return
		}
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write([]byte("MP3BYTES"))
	}))
	defer rich.Close()
	speaking := New(rich.URL, "")
	text, err := speaking.Transcribe(ctx, "m", "a.wav", []byte("x"))
	if err != nil || text != "hello there" {
		t.Fatalf("transcribe %q err %v", text, err)
	}
	audio, contentType, err := speaking.Speak(ctx, "m", "alloy", "hi")
	if err != nil || string(audio) != "MP3BYTES" || contentType != "audio/mpeg" {
		t.Fatalf("speak %q %q err %v", audio, contentType, err)
	}
}
