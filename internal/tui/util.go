package tui

import "strings"

func truncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= w {
		return s
	}
	if w == 1 {
		return string(r[:1])
	}
	return string(r[:w-1]) + "…"
}

func padRight(s string, w int) string {
	d := w - len([]rune(s))
	if d <= 0 {
		return s
	}
	return s + strings.Repeat(" ", d)
}

// wrapText hard-wraps s to width w, honouring existing newlines.
func wrapText(s string, w int) []string {
	if w <= 0 {
		return []string{s}
	}
	var out []string
	for _, para := range strings.Split(s, "\n") {
		if strings.TrimSpace(para) == "" {
			out = append(out, "")
			continue
		}
		line := ""
		for _, word := range strings.Fields(para) {
			switch {
			case line == "":
				line = word
			case len([]rune(line))+1+len([]rune(word)) <= w:
				line += " " + word
			default:
				out = append(out, line)
				line = word
			}
			for len([]rune(line)) > w {
				r := []rune(line)
				out = append(out, string(r[:w]))
				line = string(r[w:])
			}
		}
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}
