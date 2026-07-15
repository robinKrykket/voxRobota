package tui

import "testing"

func TestMatchSpokenChoice(t *testing.T) {
	items := []string{"Use Postgres", "Use SQLite", "Something else"}
	cases := []struct {
		in     string
		want   int
		wantOK bool
	}{
		{"two", 1, true},
		{"2", 1, true},
		{"option 3", 2, true},
		{"use sqlite", 1, true},
		{"postgres", 0, true},
		{"the second one", 1, true},
		{"make me a sandwich", -1, false},
	}
	for _, c := range cases {
		got, ok := matchSpokenChoice(c.in, items)
		if ok != c.wantOK || (ok && got != c.want) {
			t.Errorf("matchSpokenChoice(%q) = (%d,%v), want (%d,%v)", c.in, got, ok, c.want, c.wantOK)
		}
	}
}

func TestMatchSpokenChoiceEmpty(t *testing.T) {
	if _, ok := matchSpokenChoice("two", nil); ok {
		t.Error("expected no match with empty items")
	}
}

func TestParseNum(t *testing.T) {
	for in, want := range map[string]int{"one": 1, "nine": 9, "third": 3, "5": 5, "banana": 0} {
		if got := parseNum(in); got != want {
			t.Errorf("parseNum(%q) = %d, want %d", in, got, want)
		}
	}
}
