package session

import "fmt"

// Stats tracks the running context/cost picture for the session. Context
// tokens reflect the *current* conversation size (input + cache on the last
// turn), so they're replaced each turn; cost accumulates across turns.
type Stats struct {
	ContextTokens int
	ContextWindow int
	DefaultWindow int // fallback when a turn omits modelUsage
	CostUSD       float64
	Turns         int
}

// Update folds in one finished turn.
func (s *Stats) Update(contextTokens, contextWindow int, costUSD float64) {
	if contextTokens > 0 {
		s.ContextTokens = contextTokens
	}
	if contextWindow > 0 {
		s.ContextWindow = contextWindow
	}
	s.CostUSD += costUSD
	s.Turns++
}

// Window returns the effective context window (with fallback).
func (s *Stats) Window() int {
	if s.ContextWindow > 0 {
		return s.ContextWindow
	}
	return s.DefaultWindow
}

// Pct is the context-fill fraction in 0..1.
func (s *Stats) Pct() float64 {
	w := s.Window()
	if w == 0 {
		return 0
	}
	return float64(s.ContextTokens) / float64(w)
}

// ShouldCompact reports whether context fill has crossed the threshold.
func (s *Stats) ShouldCompact(threshold float64) bool {
	return threshold > 0 && s.Pct() >= threshold
}

// ResetContext clears the context estimate after a compaction; it refills on
// the next turn's result.
func (s *Stats) ResetContext() { s.ContextTokens = 0 }

// HumanTokens renders a token count like "18.4k".
func HumanTokens(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fk", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}
