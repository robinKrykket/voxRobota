package tui

import (
	"fmt"
	"strings"
)

// Choices is the selectable list parsed from Claude's ### Choices section.
type Choices struct {
	items  []string
	cursor int
}

func (c *Choices) Set(items []string) { c.items = items; c.cursor = 0 }
func (c *Choices) Clear()             { c.items = nil; c.cursor = 0 }
func (c *Choices) Len() int           { return len(c.items) }

func (c *Choices) Move(d int) {
	if len(c.items) == 0 {
		return
	}
	c.cursor = (c.cursor + d + len(c.items)) % len(c.items)
}

func (c *Choices) Selected() (string, bool) {
	if len(c.items) == 0 {
		return "", false
	}
	return c.items[c.cursor], true
}

func (c *Choices) At(i int) (string, bool) {
	if i < 0 || i >= len(c.items) {
		return "", false
	}
	return c.items[i], true
}

// RowAt maps a row offset within the rendered list to a choice index (-1 if
// out of range). Row 0 is the header, choices start at row 1.
func (c *Choices) RowAt(row int) int {
	idx := row - 1
	if idx < 0 || idx >= len(c.items) {
		return -1
	}
	return idx
}

func (c *Choices) View(width int) string {
	if len(c.items) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(subtitleStyle.Render("choices") + "\n")
	for i, it := range c.items {
		line := fmt.Sprintf("%d. %s", i+1, it)
		if i == c.cursor {
			b.WriteString(selectedRow.Render(" "+line+" ") + "\n")
		} else {
			b.WriteString(" " + youStyle.Render(fmt.Sprintf("%d.", i+1)) + " " + textStyle.Render(it) + "\n")
		}
	}
	return b.String()
}

var numberWords = map[string]int{
	"one": 1, "two": 2, "three": 3, "four": 4, "five": 5,
	"six": 6, "seven": 7, "eight": 8, "nine": 9,
	"first": 1, "second": 2, "third": 3, "fourth": 4, "fifth": 5,
	"sixth": 6, "seventh": 7, "eighth": 8, "ninth": 9,
}

// matchSpokenChoice tries to resolve spoken text to a choice index. Returns
// (index, true) on a confident match, else (-1, false) so the caller can
// treat the utterance as a fresh prompt.
func matchSpokenChoice(text string, items []string) (int, bool) {
	if len(items) == 0 {
		return -1, false
	}
	norm := strings.ToLower(strings.TrimSpace(text))
	norm = strings.Trim(norm, ".!?, ")

	// Digit or number word anywhere in a short utterance ("two", "option 2").
	for _, w := range strings.Fields(norm) {
		w = strings.Trim(w, ".!?,")
		if n := parseNum(w); n >= 1 && n <= len(items) {
			return n - 1, true
		}
	}

	// Text overlap against option labels.
	best, bestScore := -1, 0.0
	spokenSet := wordSet(norm)
	for i, it := range items {
		low := strings.ToLower(it)
		if norm != "" && (strings.Contains(low, norm) || strings.Contains(norm, low)) {
			return i, true
		}
		score := overlap(spokenSet, wordSet(low))
		if score > bestScore {
			best, bestScore = i, score
		}
	}
	if bestScore >= 0.5 {
		return best, true
	}
	return -1, false
}

func parseNum(w string) int {
	if n, ok := numberWords[w]; ok {
		return n
	}
	if len(w) == 1 && w[0] >= '1' && w[0] <= '9' {
		return int(w[0] - '0')
	}
	return 0
}

func wordSet(s string) map[string]bool {
	set := map[string]bool{}
	for _, w := range strings.Fields(s) {
		w = strings.Trim(w, ".!?,'\"")
		if len(w) > 1 {
			set[w] = true
		}
	}
	return set
}

func overlap(a, b map[string]bool) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	inter := 0
	for w := range a {
		if b[w] {
			inter++
		}
	}
	smaller := len(a)
	if len(b) < smaller {
		smaller = len(b)
	}
	return float64(inter) / float64(smaller)
}
