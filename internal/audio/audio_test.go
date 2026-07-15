package audio

import (
	"bytes"
	"testing"
)

func TestWAVRoundTrip(t *testing.T) {
	pcm := make([]byte, 0, 200)
	for i := 0; i < 100; i++ {
		pcm = append(pcm, byte(i), byte(i>>8))
	}
	wav := EncodeWAV(pcm, 16000, 1)
	got, rate, ch, err := DecodeWAV(wav)
	if err != nil {
		t.Fatal(err)
	}
	if rate != 16000 || ch != 1 {
		t.Fatalf("rate=%d ch=%d", rate, ch)
	}
	if !bytes.Equal(got, pcm) {
		t.Fatalf("pcm mismatch: got %d bytes want %d", len(got), len(pcm))
	}
}

func TestLevelsFromPCM(t *testing.T) {
	// Loud full-scale samples should yield high bars; silence yields zero.
	loud := make([]byte, 320)
	for i := 0; i < len(loud); i += 2 {
		loud[i], loud[i+1] = 0xFF, 0x7F // ~ max positive int16
	}
	lv := levelsFromPCM(loud, 8)
	if len(lv) != 8 {
		t.Fatalf("want 8 bars, got %d", len(lv))
	}
	if lv[0] < 0.5 {
		t.Errorf("expected loud bar, got %v", lv[0])
	}
	silent := levelsFromPCM(make([]byte, 320), 8)
	if silent[0] != 0 {
		t.Errorf("expected silence 0, got %v", silent[0])
	}
}
