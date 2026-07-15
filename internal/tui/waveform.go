package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var waveBlocks = []rune{' ', '▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

// waveRows is the height of the waveform strip in rows.
const waveRows = 2

// renderWaveTall draws a `rows`-tall waveform: each column's level fills from
// the bottom row up, using eighth-blocks for the partial top cell.
func renderWaveTall(levels []float64, width, rows int, color lipgloss.Color) string {
	if width <= 0 || rows <= 0 {
		return ""
	}
	n := len(levels)
	lines := make([]string, rows)
	for r := 0; r < rows; r++ { // r = 0 is the top row
		fromBottom := rows - 1 - r // 0 = bottom row
		bar := make([]rune, width)
		for i := 0; i < width; i++ {
			var l float64
			if n > 0 {
				l = levels[i*n/width]
			}
			eighths := int(l*float64(rows*8) + 0.5) // total filled eighths in this column
			cell := eighths - fromBottom*8
			if cell < 0 {
				cell = 0
			}
			if cell > 8 {
				cell = 8
			}
			bar[i] = waveBlocks[cell]
		}
		lines[r] = lipgloss.NewStyle().Foreground(color).Render(string(bar))
	}
	return strings.Join(lines, "\n")
}

// flatWaveTall is the idle baseline: blank rows above a thin bottom line.
func flatWaveTall(width, rows int, color lipgloss.Color) string {
	if width <= 0 || rows <= 0 {
		return ""
	}
	lines := make([]string, rows)
	for r := 0; r < rows-1; r++ {
		lines[r] = strings.Repeat(" ", width)
	}
	lines[rows-1] = lipgloss.NewStyle().Foreground(color).Render(strings.Repeat("▁", width))
	return strings.Join(lines, "\n")
}

// renderWave draws a single-row neon waveform of the given levels across
// width columns, in the given color. levels is resampled to fit width.
func renderWave(levels []float64, width int, color lipgloss.Color) string {
	if width <= 0 {
		return ""
	}
	bars := make([]rune, width)
	n := len(levels)
	for i := 0; i < width; i++ {
		var l float64
		if n > 0 {
			l = levels[i*n/width]
		}
		idx := int(l*float64(len(waveBlocks)-1) + 0.5)
		if idx < 0 {
			idx = 0
		}
		if idx > len(waveBlocks)-1 {
			idx = len(waveBlocks) - 1
		}
		bars[i] = waveBlocks[idx]
	}
	return lipgloss.NewStyle().Foreground(color).Render(string(bars))
}

// flatWave is the idle baseline.
func flatWave(width int, color lipgloss.Color) string {
	if width <= 0 {
		return ""
	}
	return lipgloss.NewStyle().Foreground(color).Render(strings.Repeat("▁", width))
}
