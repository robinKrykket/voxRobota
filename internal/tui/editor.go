package tui

import (
	"os"
	"path/filepath"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
)

// Editor is a modal, keyboard-only text editor with vim-ish modes:
// normal (navigate) and insert. Ctrl+S saves & closes; Esc/q in normal closes.
type Editor struct {
	ta     textarea.Model
	path   string
	insert bool
	dirty  bool
	loaded string
	width  int
	height int
	status string

	Done  bool // set when the editor should close
	Saved bool
}

func NewEditor() Editor {
	ta := textarea.New()
	ta.Prompt = ""
	ta.ShowLineNumbers = true
	ta.CharLimit = 0
	return Editor{ta: ta}
}

// Open loads a file into the editor in normal mode.
func (e *Editor) Open(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	e.path = path
	e.ta.SetValue(string(data))
	e.loaded = string(data)
	e.insert = false
	e.dirty = false
	e.Done = false
	e.Saved = false
	e.status = "NORMAL · i edit · ctrl+s save · esc close"
	e.ta.Blur()
	return nil
}

func (e *Editor) SetSize(w, h int) {
	e.width = w
	e.height = h
	e.ta.SetWidth(max(10, w-4))
	e.ta.SetHeight(max(3, h-5))
}

func (e *Editor) save() {
	if err := os.WriteFile(e.path, []byte(e.ta.Value()), 0o644); err != nil {
		e.status = "save error: " + err.Error()
		return
	}
	e.dirty = false
	e.Saved = true
	e.Done = true
}

// Update routes a key to the editor. Returns the updated editor and any cmd.
func (e Editor) Update(msg tea.KeyMsg) (Editor, tea.Cmd) {
	if msg.String() == "ctrl+s" {
		e.save()
		return e, nil
	}

	if e.insert {
		if msg.Type == tea.KeyEsc {
			e.insert = false
			e.ta.Blur()
			e.status = "NORMAL · i edit · ctrl+s save · esc close"
			return e, nil
		}
		var cmd tea.Cmd
		e.ta, cmd = e.ta.Update(msg)
		if e.ta.Value() != e.loaded {
			e.dirty = true
		}
		return e, cmd
	}

	// normal mode
	switch msg.String() {
	case "i":
		e.insert = true
		e.status = "INSERT · esc normal · ctrl+s save"
		return e, e.ta.Focus()
	case "esc", "q":
		e.Done = true
		return e, nil
	case "up", "down", "left", "right", "pgup", "pgdown", "home", "end":
		var cmd tea.Cmd
		e.ta, cmd = e.ta.Update(msg)
		return e, cmd
	}
	return e, nil
}

func (e Editor) View() string {
	name := filepath.Base(e.path)
	header := subtitleStyle.Render(name)
	if e.dirty {
		header += claudeStyle.Render(" " + glyphDirty)
	}
	modeTag := subtitleStyle.Render("NORMAL")
	if e.insert {
		modeTag = claudeStyle.Render("INSERT")
	}
	content := header + "  " + modeTag + "\n" + e.ta.View() + "\n" + dimStyle.Render(e.status)
	return modalBorder.Width(max(12, e.width-2)).Render(content)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
