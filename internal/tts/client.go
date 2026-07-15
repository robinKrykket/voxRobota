package tts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Client talks to the Python sidecar's /tts endpoint. We POST text and get
// back a WAV blob (24 kHz mono s16 from Kokoro).
type Client struct {
	BaseURL string
	Voice   string
	HTTP    *http.Client
}

type ttsRequest struct {
	Text  string `json:"text"`
	Voice string `json:"voice,omitempty"`
}

// Synthesize returns WAV audio bytes for the given text.
func (c *Client) Synthesize(ctx context.Context, text string) ([]byte, error) {
	payload, _ := json.Marshal(ttsRequest{Text: text, Voice: c.Voice})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/tts", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("tts request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tts sidecar %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}

func (c *Client) client() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}
