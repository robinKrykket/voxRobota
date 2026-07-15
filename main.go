package main

import (
	"fmt"
	"net/http"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"voxrobota/internal/audio"
	"voxrobota/internal/claude"
	"voxrobota/internal/config"
	"voxrobota/internal/stt"
	"voxrobota/internal/tts"
	"voxrobota/internal/tui"
)

func main() {
	cfg := config.Load()

	engine, err := audio.NewEngine()
	if err != nil {
		fmt.Fprintln(os.Stderr, "audio init failed:", err)
		os.Exit(1)
	}
	defer engine.Close()

	rec := audio.NewRecorder(engine, cfg.CaptureSampleRate)
	player := audio.NewPlayer(engine)

	httpClient := &http.Client{Timeout: 120 * time.Second}
	sttC := &stt.Client{BaseURL: cfg.SidecarURL, HTTP: httpClient}
	ttsC := &tts.Client{BaseURL: cfg.SidecarURL, Voice: cfg.Voice, HTTP: httpClient}
	cl := &claude.Client{
		Bin:           cfg.ClaudeBin,
		SystemPrompt:  cfg.SystemPrompt,
		SpokenMarker:  cfg.SpokenMarker,
		ChoicesMarker: cfg.ChoicesMarker,
	}

	warnIfSidecarDown(cfg.SidecarURL)

	model := tui.New(cfg, rec, player, cl, sttC, ttsC)
	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func warnIfSidecarDown(base string) {
	c := &http.Client{Timeout: 2 * time.Second}
	resp, err := c.Get(base + "/health")
	if err != nil || resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "warning: STT/TTS sidecar not reachable at %s — start it with `make sidecar`\n", base)
		time.Sleep(1200 * time.Millisecond)
		return
	}
	resp.Body.Close()
}
