package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"voxrobota/internal/session"
)

// renderStatusBar draws the bottom bar: mode, model, context tokens/%, cost,
// sidecar health.
func renderStatusBar(width int, modeName, modelLabel string, st session.Stats, sidecarOK bool) string {
	modeBadge := lipgloss.NewStyle().
		Foreground(modeColor(modeName)).
		Render(modeName)

	pct := st.Pct() * 100
	tok := fmt.Sprintf("ctx %s/%s %.0f%%",
		session.HumanTokens(st.ContextTokens), session.HumanTokens(st.Window()), pct)
	tokStyle := dimStyle
	if pct >= 80 {
		tokStyle = lipgloss.NewStyle().Foreground(colYellow)
	}

	scDot := lipgloss.NewStyle().Foreground(colGreen).Render("●")
	if !sidecarOK {
		scDot = lipgloss.NewStyle().Foreground(colPink).Render("●")
	}

	segs := []string{
		modeBadge,
		subtitleStyle.Render(modelLabel),
		tokStyle.Render(tok),
		scDot + dimStyle.Render(" sidecar"),
	}
	line := strings.Join(segs, dimStyle.Render("  ·  "))

	return lipgloss.NewStyle().
		Background(colPanel).
		Width(width).
		Render(" " + line)
}
