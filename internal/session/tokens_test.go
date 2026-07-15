package session

import "testing"

func TestStatsUpdateAndPct(t *testing.T) {
	s := Stats{DefaultWindow: 200000}
	s.Update(160000, 200000, 0.05)
	s.Update(180000, 200000, 0.03) // context replaced, cost accumulates
	if s.ContextTokens != 180000 {
		t.Errorf("ContextTokens = %d", s.ContextTokens)
	}
	if s.Window() != 200000 {
		t.Errorf("Window = %d", s.Window())
	}
	if got := s.CostUSD; got < 0.079 || got > 0.081 {
		t.Errorf("CostUSD = %v, want ~0.08", got)
	}
	if !s.ShouldCompact(0.80) {
		t.Errorf("expected ShouldCompact at 90%% fill")
	}
	if s.ShouldCompact(0.95) {
		t.Errorf("did not expect compaction below 95%%")
	}
}

func TestResetContext(t *testing.T) {
	s := Stats{DefaultWindow: 1000}
	s.Update(900, 1000, 0.01)
	s.ResetContext()
	if s.Pct() != 0 {
		t.Errorf("Pct after reset = %v", s.Pct())
	}
}

func TestHumanTokens(t *testing.T) {
	for in, want := range map[int]string{500: "500", 18400: "18.4k", 1_500_000: "1.5M"} {
		if got := HumanTokens(in); got != want {
			t.Errorf("HumanTokens(%d) = %q, want %q", in, got, want)
		}
	}
}
