package tui

import "github.com/charmbracelet/lipgloss"

// Active palette. These are variables (not consts) so /theme can swap them at
// runtime; applyTheme rebuilds every derived style below.
var (
	colBg     lipgloss.Color
	colPanel  lipgloss.Color
	colText   lipgloss.Color
	colDim    lipgloss.Color
	colPink   lipgloss.Color
	colBlue   lipgloss.Color
	colCyan   lipgloss.Color
	colPurple lipgloss.Color
	colGreen  lipgloss.Color
	colYellow lipgloss.Color
)

// Flat glyphs — no emoji.
const (
	glyphUser   = "●" // you
	glyphClaude = "▼" // claude
	glyphIdle   = "○"
	glyphAttach = "▸"
	glyphChoice = "❯"
	glyphDirty  = "●"
)

var (
	titleStyle    lipgloss.Style
	subtitleStyle lipgloss.Style
	dimStyle      lipgloss.Style
	textStyle     lipgloss.Style
	youStyle      lipgloss.Style
	claudeStyle   lipgloss.Style
	toolStyle     lipgloss.Style

	focusBorder lipgloss.Style
	blurBorder  lipgloss.Style
	selectedRow lipgloss.Style
	modalBorder lipgloss.Style
)

// themeDef is a full palette + the accent choices derived styles need.
type themeDef struct {
	bg, panel, text, dim                     lipgloss.Color
	pink, blue, cyan, purple, green, yellow  lipgloss.Color
	focusBdr, blurBdr, modalBdr              lipgloss.Color
	selFg, selBg                             lipgloss.Color
}

var themes = map[string]themeDef{
	// Miami Vice — soft neon on deep violet.
	"vice": {
		bg: "#17131F", panel: "#221A33", text: "#D7CDEB", dim: "#6C6685",
		pink: "#FF6AD5", blue: "#4EA8FF", cyan: "#54E6D6", purple: "#9D7CD8",
		green: "#5BE3B3", yellow: "#F4D06F",
		focusBdr: "#FF6AD5", blurBdr: "#9D7CD8", modalBdr: "#9D7CD8",
		selFg: "#17131F", selBg: "#FF6AD5",
	},
	// Berserk — black, red, and bone white. Blue/purple → red; pink → white;
	// every panel and modal outline red; the selection highlighter white.
	"berserk": {
		bg: "#0E0A0C", panel: "#1A0E10", text: "#EDE6E6", dim: "#7A6A6C",
		pink: "#F5F5F5", blue: "#E01F28", cyan: "#FF4D4D", purple: "#B3141C",
		green: "#5BE3B3", yellow: "#F4D06F",
		focusBdr: "#E01F28", blurBdr: "#B3141C", modalBdr: "#E01F28",
		selFg: "#0E0A0C", selBg: "#F5F5F5",
	},
}

var activeThemeName = "vice"

func themeNames() []string { return []string{"vice", "berserk"} }

// applyTheme swaps the palette and rebuilds all derived styles.
func applyTheme(name string) bool {
	t, ok := themes[name]
	if !ok {
		return false
	}
	activeThemeName = name

	colBg, colPanel, colText, colDim = t.bg, t.panel, t.text, t.dim
	colPink, colBlue, colCyan, colPurple = t.pink, t.blue, t.cyan, t.purple
	colGreen, colYellow = t.green, t.yellow

	titleStyle = lipgloss.NewStyle().Foreground(colPink)
	subtitleStyle = lipgloss.NewStyle().Foreground(colPurple)
	dimStyle = lipgloss.NewStyle().Foreground(colDim)
	textStyle = lipgloss.NewStyle().Foreground(colText)
	youStyle = lipgloss.NewStyle().Foreground(colPurple) // your messages: purple (vice) / red (berserk)
	claudeStyle = lipgloss.NewStyle().Foreground(colPink)
	toolStyle = lipgloss.NewStyle().Foreground(colPurple)

	focusBorder = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(t.focusBdr)
	blurBorder = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(t.blurBdr)
	selectedRow = lipgloss.NewStyle().Foreground(t.selFg).Background(t.selBg)
	modalBorder = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(t.modalBdr).Padding(1, 2)
	return true
}

func init() { applyTheme("vice") }

func modeColor(name string) lipgloss.Color {
	switch name {
	case "PLAN":
		return colCyan
	case "AUTO":
		return colGreen
	case "MANUAL":
		return colPink
	}
	return colDim
}

func border(focused bool) lipgloss.Style {
	if focused {
		return focusBorder
	}
	return blurBorder
}
