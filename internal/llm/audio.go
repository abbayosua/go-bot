package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
)

// Transcribe sends audio to an OpenAI-compatible transcriptions endpoint and
// returns the text. Providers without audio support fail openly so the UI can
// say dictation is unavailable instead of pretending.
func (c *Client) Transcribe(ctx context.Context, model, filename string, audio []byte) (string, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("model", model)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return "", err
	}
	if _, err := part.Write(audio); err != nil {
		return "", err
	}
	if err := writer.Close(); err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/audio/transcriptions", &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusBadRequest {
		return "", fmt.Errorf("provider has no audio transcription (status %d)", resp.StatusCode)
	}
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("transcribe status %d: %.200s", resp.StatusCode, raw)
	}
	var out struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || strings.TrimSpace(out.Text) == "" {
		return "", fmt.Errorf("transcribe decode failed: %.200s", raw)
	}
	return strings.TrimSpace(out.Text), nil
}

// Speak sends text to an OpenAI-compatible speech endpoint and returns audio
// bytes with the provider's content type. Same fail-open contract.
func (c *Client) Speak(ctx context.Context, model, voice, text string) ([]byte, string, error) {
	payload, _ := json.Marshal(map[string]any{"model": model, "voice": voice, "input": text})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/audio/speech", bytes.NewReader(payload))
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, "", err
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusBadRequest {
		return nil, "", fmt.Errorf("provider has no speech synthesis (status %d)", resp.StatusCode)
	}
	if resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("speak status %d: %.200s", resp.StatusCode, raw)
	}
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "audio/mpeg"
	}
	return raw, contentType, nil
}
