package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// helpLines documents every keybinding; shown in the ? overlay and hinted in
// the footer.
var helpLines = [][2]string{
	{"space", "talk / stop / interrupt (push-to-talk toggle)"},
	{"shift+tab", "cycle mode: PLAN → AUTO → MANUAL"},
	{"tab", "cycle focus: tree ↔ conversation ↔ choices"},
	{"1-9", "pick a numbered choice"},
	{"↑ ↓ ← →", "navigate focused panel"},
	{"enter", "select / open file / expand dir"},
	{"t", "type a message to Claude"},
	{"/", "commands: /model /mode /theme /compact /new /help /quit"},
	{"drag image", "attach image file to next message"},
	{"F2", "model picker"},
	{"ctrl+k", "compact context now"},
	{"ctrl+n", "new window here (cwd)"},
	{"ctrl+g", "new window at home"},
	{"i / esc / ctrl+s", "editor: insert / normal / save & close"},
	{"?", "toggle this help"},
	{"q / ctrl+c", "quit"},
}

func helpOverlay(width, height int) string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("voxRobota — keybindings") + "\n\n")
	keyStyle := lipgloss.NewStyle().Foreground(colCyan).Width(18)
	for _, kv := range helpLines {
		b.WriteString(keyStyle.Render(kv[0]) + dimStyle.Render(kv[1]) + "\n")
	}
	b.WriteString("\n" + dimStyle.Render("press ? or esc to close"))
	return modalBorder.Render(b.String())
}

func footerHint() string {
	return dimStyle.Render("space talk · shift+tab mode · tab focus · 1-9 choose · F2 model · ctrl+k compact · ? help · q quit")
}
