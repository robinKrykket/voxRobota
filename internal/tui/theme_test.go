package tui

import "testing"

func TestThemeSwitch(t *testing.T) {
	defer applyTheme("vice")

	if !applyTheme("berserk") {
		t.Fatal("berserk theme should exist")
	}
	// Waveform + glyph colors: mic uses colBlue, Claude uses colPink.
	if string(colBlue) != "#E01F28" {
		t.Errorf("berserk voice color = %s, want red #E01F28", colBlue)
	}
	if string(colPink) != "#F5F5F5" {
		t.Errorf("berserk AI color = %s, want white #F5F5F5", colPink)
	}
	if string(colPurple) != "#B3141C" {
		t.Errorf("berserk purple should be red, got %s", colPurple)
	}

	if applyTheme("does-not-exist") {
		t.Error("unknown theme should return false")
	}

	applyTheme("vice")
	if string(colBlue) != "#4EA8FF" {
		t.Errorf("vice voice color = %s, want blue", colBlue)
	}
}
