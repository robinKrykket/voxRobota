package claude

import "testing"

func newTestClient() *Client {
	return &Client{SpokenMarker: "### Spoken", ChoicesMarker: "### Choices"}
}

func TestExtractChoices(t *testing.T) {
	c := newTestClient()
	full := "Did some analysis.\n\n### Choices\n1. Use Postgres\n2. Use SQLite\n3. Something else\n\n### Spoken\nI looked at the options. Which database should we use?"
	got := c.extractChoices(full)
	want := []string{"Use Postgres", "Use SQLite", "Something else"}
	if len(got) != len(want) {
		t.Fatalf("got %d choices, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("choice %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestExtractSpoken(t *testing.T) {
	c := newTestClient()
	full := "Body text.\n\n### Spoken\nHere is the summary. What next?"
	if got := c.extractSpoken(full); got != "Here is the summary. What next?" {
		t.Errorf("spoken = %q", got)
	}
}

func TestExtractSpokenFallback(t *testing.T) {
	c := newTestClient()
	if got := c.extractSpoken("no markers here"); got != "no markers here" {
		t.Errorf("fallback spoken = %q", got)
	}
}

func TestCleanDisplay(t *testing.T) {
	full := "Here is the work I did.\n\n### Choices\n1. A\n2. B\n\n### Spoken\nSpoken words only heard."
	got := CleanDisplay(full, "### Spoken", "### Choices")
	if got != "Here is the work I did." {
		t.Errorf("CleanDisplay = %q", got)
	}
	// A non-marker heading in the body must survive.
	full2 := "Intro.\n\n### Notes\nsome notes\n\n### Spoken\nspoken."
	got2 := CleanDisplay(full2, "### Spoken", "### Choices")
	if got2 != "Intro.\n\n### Notes\nsome notes" {
		t.Errorf("CleanDisplay preserved-heading = %q", got2)
	}
}

func TestExtractChoicesNone(t *testing.T) {
	c := newTestClient()
	if got := c.extractChoices("just a reply\n\n### Spoken\nhi"); got != nil {
		t.Errorf("expected no choices, got %v", got)
	}
}
