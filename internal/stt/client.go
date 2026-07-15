package stt

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Client talks to the Python sidecar's /stt endpoint. We POST a WAV blob
// and get back the transcript.
type Client struct {
	BaseURL string
	HTTP    *http.Client
}

type sttResponse struct {
	Text string `json:"text"`
}

// Transcribe sends 16 kHz mono PCM wrapped as a WAV and returns the text.
func (c *Client) Transcribe(ctx context.Context, wav []byte) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/stt", bytes.NewReader(wav))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "audio/wav")

	resp, err := c.client().Do(req)
	if err != nil {
		return "", fmt.Errorf("stt request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("stt sidecar %d: %s", resp.StatusCode, string(body))
	}

	var r sttResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return "", fmt.Errorf("stt decode: %w", err)
	}
	return r.Text, nil
}

func (c *Client) client() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}
