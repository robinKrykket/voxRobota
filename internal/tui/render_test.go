package tui

import (
	"os"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"voxrobota/internal/claude"
	"voxrobota/internal/config"
)

// TestRenderSnapshot builds the UI headlessly and prints View() so the layout
// and glyphs can be eyeballed. Run: VOX_RENDER=1 go test ./internal/tui -run Snapshot -v
func TestRenderSnapshot(t *testing.T) {
	if os.Getenv("VOX_RENDER") == "" {
		t.Skip("set VOX_RENDER=1 to print a UI snapshot")
	}
	cfg := config.Load()
	cl := &claude.Client{SpokenMarker: cfg.SpokenMarker, ChoicesMarker: cfg.ChoicesMarker}
	m := New(cfg, nil, nil, cl, nil, nil)

	tm, _ := m.Update(tea.WindowSizeMsg{Width: 108, Height: 32})
	m = tm.(Model)
	m.transcript = []convLine{
		{glyph: glyphUser, glyphStyle: youStyle, text: "can you refactor the token parser?"},
		{glyph: glyphClaude, glyphStyle: claudeStyle, text: "Done — I split it into scan, parse, and validate, and added tests. What next?"},
	}
	m.choices.Set([]string{"Run the tests", "Review the diff", "Something else"})
	m.stats.ContextTokens = 184000
	m.stats.ContextWindow = 1000000
	m.stats.CostUSD = 0.42
	m.sidecarOK = true
	(&m).relayout()

	t.Log("\n" + m.View())
}
