package tui

import (
	"fmt"
	"strings"

	"voxrobota/internal/config"
)

// ModelPicker is the F2 overlay for choosing the active model.
type ModelPicker struct {
	models []config.Model
	cursor int
}

func (m *ModelPicker) Open(models []config.Model, currentAlias string) {
	m.models = models
	m.cursor = 0
	for i, md := range models {
		if md.Alias == currentAlias {
			m.cursor = i
			break
		}
	}
}

func (m *ModelPicker) Move(d int) {
	if len(m.models) == 0 {
		return
	}
	m.cursor = (m.cursor + d + len(m.models)) % len(m.models)
}

func (m *ModelPicker) Selected() (config.Model, bool) {
	if m.cursor < 0 || m.cursor >= len(m.models) {
		return config.Model{}, false
	}
	return m.models[m.cursor], true
}

// RowAt maps a rendered row (0 = header) to a model index, or -1.
func (m *ModelPicker) RowAt(row int) int {
	idx := row - 2 // title + blank line
	if idx < 0 || idx >= len(m.models) {
		return -1
	}
	return idx
}

func (m *ModelPicker) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("select model") + "\n\n")
	for i, md := range m.models {
		label := fmt.Sprintf("%s  %s", md.Alias, dimStyle.Render(md.Label))
		if i == m.cursor {
			b.WriteString(selectedRow.Render(" "+md.Alias+"  "+md.Label+" ") + "\n")
		} else {
			b.WriteString(" " + textStyle.Render(label) + "\n")
		}
	}
	b.WriteString("\n" + dimStyle.Render("↑↓ move · enter select · esc cancel"))
	return modalBorder.Render(b.String())
}
