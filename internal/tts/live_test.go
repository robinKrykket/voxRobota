package tts

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"voxrobota/internal/stt"
)

// TestLiveRoundTrip exercises the real Go STT+TTS clients against a running
// sidecar. Skipped unless VOX_LIVE=1 (and the sidecar is up on :8123).
func TestLiveRoundTrip(t *testing.T) {
	if os.Getenv("VOX_LIVE") == "" {
		t.Skip("set VOX_LIVE=1 with the sidecar running to exercise the live clients")
	}
	base := "http://127.0.0.1:8123"
	hc := &http.Client{Timeout: 60 * time.Second}

	wav, err := (&Client{BaseURL: base, HTTP: hc}).Synthesize(context.Background(), "Testing the go client round trip.")
	if err != nil {
		t.Fatal("synthesize:", err)
	}
	if len(wav) < 44 {
		t.Fatalf("wav too short: %d bytes", len(wav))
	}

	text, err := (&stt.Client{BaseURL: base, HTTP: hc}).Transcribe(context.Background(), wav)
	if err != nil {
		t.Fatal("transcribe:", err)
	}
	if !strings.Contains(strings.ToLower(text), "round") {
		t.Fatalf("unexpected transcription: %q", text)
	}
	t.Logf("round-trip transcription: %q", text)
}
