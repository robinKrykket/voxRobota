package config

import (
	"os"
	"runtime"
	"strconv"
	"strings"
)

// Model is one selectable Claude model.
type Model struct {
	Alias string // passed to --model, e.g. "opus"
	Label string // shown in the picker / status bar
}

// Mode is a user-facing permission mode cycled with shift+tab.
type Mode struct {
	Name       string // shown in the status bar: PLAN / AUTO / MANUAL
	Permission string // value passed to --permission-mode
}

// Modes are cycled in this order with shift+tab.
var Modes = []Mode{
	{Name: "PLAN", Permission: "plan"},
	{Name: "AUTO", Permission: "auto"},
	{Name: "MANUAL", Permission: "default"},
}

// Config holds runtime settings. Everything has a sane default so the app
// runs with zero configuration; override via environment variables.
type Config struct {
	SidecarURL         string
	ClaudeBin          string
	CaptureSampleRate  uint32
	PlaybackSampleRate uint32
	Voice              string

	SpokenMarker  string // heading whose text is spoken aloud
	ChoicesMarker string // heading that introduces the numbered choice list
	SystemPrompt  string // appended to Claude's system prompt

	Models       []Model
	DefaultModel string // alias

	// CompactAtPct is the context-fill fraction (0..1) at which we offer /
	// trigger emulated compaction.
	CompactAtPct float64
	// DefaultContextWindow is used only if the result event omits modelUsage.
	DefaultContextWindow int

	// TerminalTemplate builds the new-window command; {dir} and {bin} are
	// substituted at spawn time.
	TerminalTemplate []string

	HomeDir string
}

func Load() Config {
	home, _ := os.UserHomeDir()
	c := Config{
		SidecarURL:           env("VOX_SIDECAR_URL", "http://127.0.0.1:8123"),
		ClaudeBin:            env("VOX_CLAUDE_BIN", "claude"),
		CaptureSampleRate:    uint32(envInt("VOX_CAPTURE_RATE", 16000)),
		PlaybackSampleRate:   uint32(envInt("VOX_PLAYBACK_RATE", 24000)),
		Voice:                env("VOX_VOICE", "af_heart"),
		SpokenMarker:         "### Spoken",
		ChoicesMarker:        "### Choices",
		DefaultModel:         env("VOX_MODEL", "opus"),
		CompactAtPct:         envFloat("VOX_COMPACT_AT", 0.80),
		DefaultContextWindow: envInt("VOX_CONTEXT_WINDOW", 200000),
		HomeDir:              home,
		Models: []Model{
			{Alias: "opus", Label: "Opus 4.8 (1M)"},
			{Alias: "sonnet", Label: "Sonnet 5"},
			{Alias: "haiku", Label: "Haiku 4.5"},
			{Alias: "fable", Label: "Fable 5"},
		},
		TerminalTemplate: defaultTerminalTemplate(),
	}
	c.SystemPrompt = "You are being used through a hands-free voice interface. The user " +
		"speaks to you and hears a spoken summary of your reply. Do your normal work " +
		"fully, but format the END of every response as follows.\n\n" +
		"ALWAYS finish with a section that begins with the exact line '" + c.SpokenMarker +
		"' followed by 2-4 short sentences of plain prose (no markdown, no code, no " +
		"lists) that summarize what you did and clearly state the single most important " +
		"decision or question you need from the user next.\n\n" +
		"WHEN — and only when — you are offering the user a set of discrete options to " +
		"choose between, ALSO include a section beginning with the exact line '" +
		c.ChoicesMarker + "' followed by a numbered markdown list (1., 2., 3., …) where " +
		"each item is a short option label of at most 8 words. Omit the '" + c.ChoicesMarker +
		"' section entirely when there are no discrete options."
	return c
}

// defaultTerminalTemplate picks the command to open a new terminal window per
// OS. Override with VOX_TERMINAL (space-separated; {dir} and {bin} tokens).
func defaultTerminalTemplate() []string {
	if v := os.Getenv("VOX_TERMINAL"); v != "" {
		return strings.Fields(v)
	}
	switch runtime.GOOS {
	case "darwin":
		return []string{"osascript", "-e", `tell application "Terminal" to do script "cd '{dir}' && '{bin}'"`}
	case "windows":
		return []string{"cmd", "/c", "start", "voxRobota", "cmd", "/k", "cd /d {dir} && \"{bin}\""}
	default:
		return []string{"gnome-terminal", "--working-directory={dir}", "--", "{bin}"}
	}
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func envInt(k string, def int) int {
	if v := os.Getenv(k); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func envFloat(k string, def float64) float64 {
	if v := os.Getenv(k); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}
