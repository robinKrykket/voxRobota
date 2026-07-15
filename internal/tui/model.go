package tui

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"voxrobota/internal/audio"
	"voxrobota/internal/claude"
	"voxrobota/internal/config"
	"voxrobota/internal/session"
	"voxrobota/internal/spawn"
	"voxrobota/internal/stt"
	"voxrobota/internal/tts"
)

type state int

const (
	stateIdle state = iota
	stateRecording
	stateThinking
	stateSpeaking
)

type focusArea int

const (
	focusConversation focusArea = iota
	focusTree
	focusChoices
)

type overlayKind int

const (
	overlayNone overlayKind = iota
	overlayEditor
	overlayModel
	overlayHelp
)

// ---- messages ---------------------------------------------------------

type tickMsg struct{}
type healthMsg struct{ ok bool }
type sttDoneMsg struct {
	text string
	err  error
}
type streamStartedMsg struct {
	gen int
	ch  <-chan claude.Event
	err error
}
type streamEventMsg struct {
	gen int
	ev  claude.Event
	ok  bool
}
type ttsDoneMsg struct {
	gen int
	wav []byte
	err error
}
type playDoneMsg struct {
	gen int
	err error
}
type compactDoneMsg struct {
	gen     int
	summary string
	err     error
}

type convLine struct {
	glyph      string
	glyphStyle lipgloss.Style
	text       string
}

type layout struct {
	treeW        int
	treeContentH int
	convX        int
	convContentW int
	convContentH int
	convBoxH     int
	waveY        int
	choicesY     int
	choiceRows   int
	hintY        int
	mainH        int
}

// ---- model ------------------------------------------------------------

type Model struct {
	cfg    config.Config
	rec    *audio.Recorder
	player *audio.Player
	cl     *claude.Client
	stt    *stt.Client
	tts    *tts.Client

	state    state
	pending  string
	attach   []string // image attachments for the next turn
	gen      int
	ctx      context.Context
	cancel   context.CancelFunc
	liveText string
	streamCh <-chan claude.Event

	focus   focusArea
	overlay overlayKind
	tree    Tree
	editor  Editor
	picker  ModelPicker
	choices Choices
	input   textinput.Model
	typing  bool

	transcript []convLine

	modeIdx    int
	modelAlias string
	stats      session.Stats

	status    string
	sidecarOK bool
	cwd       string

	width  int
	height int
	lay    layout
}

func New(cfg config.Config, rec *audio.Recorder, player *audio.Player, cl *claude.Client, sttC *stt.Client, ttsC *tts.Client) Model {
	cwd, _ := os.Getwd()
	if cwd == "" {
		cwd = cfg.HomeDir
	}
	ti := textinput.New()
	ti.Placeholder = "type a message…"
	ti.Prompt = "› "

	// prime the Claude client with the initial model + mode
	cl.SetModel(cfg.DefaultModel)
	cl.SetPermission(config.Modes[0].Permission)

	m := Model{
		cfg:        cfg,
		rec:        rec,
		player:     player,
		cl:         cl,
		stt:        sttC,
		tts:        ttsC,
		state:      stateIdle,
		focus:      focusConversation,
		tree:       NewTree(cwd),
		editor:     NewEditor(),
		input:      ti,
		modelAlias: cfg.DefaultModel,
		stats:      session.Stats{DefaultWindow: cfg.DefaultContextWindow},
		status:     "Ready. Press space to talk.",
		cwd:        cwd,
	}
	return m
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(tickCmd(), m.healthCmd())
}

func (m Model) mode() config.Mode { return config.Modes[m.modeIdx] }

// ---- update -----------------------------------------------------------

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.relayout()
		return m, nil

	case tickMsg:
		return m, tickCmd()

	case healthMsg:
		m.sidecarOK = msg.ok
		return m, nil

	case tea.MouseMsg:
		return m.handleMouse(msg)

	case tea.KeyMsg:
		return m.handleKey(msg)

	case sttDoneMsg:
		return m.onSTT(msg)

	case streamStartedMsg:
		if msg.gen != m.gen {
			return m, nil
		}
		if msg.err != nil {
			m.status = "Claude error: " + msg.err.Error()
			m.state = stateIdle
			return m, nil
		}
		m.streamCh = msg.ch
		return m, readStreamCmd(msg.gen, m.streamCh)

	case streamEventMsg:
		return m.onStreamEvent(msg)

	case ttsDoneMsg:
		if msg.gen != m.gen {
			return m, nil
		}
		if msg.err != nil {
			m.status = "TTS error: " + msg.err.Error()
			m.sidecarOK = false
			m.state = stateIdle
			return m, nil
		}
		return m, m.playCmd(msg.gen, msg.wav)

	case playDoneMsg:
		if msg.gen != m.gen {
			return m, nil
		}
		m.state = stateIdle
		m.status = "Ready."
		// auto-compaction once we're back to idle
		if m.stats.ShouldCompact(m.cfg.CompactAtPct) {
			return m.startCompaction()
		}
		return m, nil

	case compactDoneMsg:
		if msg.gen != m.gen {
			return m, nil
		}
		if msg.err != nil {
			m.status = "Compaction error: " + msg.err.Error()
			m.state = stateIdle
			return m, nil
		}
		m.cl.ResetWithSummary(msg.summary)
		m.stats.ResetContext()
		m.addLine("↻", toolStyle, "context compacted")
		m.state = stateIdle
		m.status = "Compacted. Ready."
		return m, nil
	}

	return m, nil
}

// ---- key handling -----------------------------------------------------

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Drag-and-drop and clipboard: a paste whose text is an image path
	// becomes an attachment; anything else flows into the text input.
	if msg.Paste {
		m.handlePaste(string(msg.Runes))
		return m, nil
	}

	// Overlays capture input.
	switch m.overlay {
	case overlayEditor:
		var cmd tea.Cmd
		m.editor, cmd = m.editor.Update(msg)
		if m.editor.Done {
			if m.editor.Saved {
				m.status = "Saved " + filepath.Base(m.editorPath())
			} else {
				m.status = "Closed editor."
			}
			m.overlay = overlayNone
		}
		return m, cmd
	case overlayModel:
		switch msg.String() {
		case "up", "k":
			m.picker.Move(-1)
		case "down", "j":
			m.picker.Move(1)
		case "enter":
			if sel, ok := m.picker.Selected(); ok {
				m.modelAlias = sel.Alias
				m.cl.SetModel(sel.Alias)
				m.status = "Model → " + sel.Alias
			}
			m.overlay = overlayNone
		case "esc", "f2", "q":
			m.overlay = overlayNone
		}
		return m, nil
	case overlayHelp:
		m.overlay = overlayNone
		return m, nil
	}

	// Text input mode.
	if m.typing {
		switch msg.Type {
		case tea.KeyEnter:
			return m.submitText()
		case tea.KeyEsc:
			m.typing = false
			m.input.Blur()
			m.input.SetValue("")
			return m, nil
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}

	// Global keys.
	switch msg.String() {
	case "ctrl+c":
		m.cancelInflight()
		return m, tea.Quit
	case "q":
		if m.state == stateIdle {
			return m, tea.Quit
		}
	case "?":
		m.overlay = overlayHelp
		return m, nil
	case " ", "space":
		return m.toggleMic()
	case "shift+tab":
		m.modeIdx = (m.modeIdx + 1) % len(config.Modes)
		m.cl.SetPermission(m.mode().Permission)
		m.status = "Mode → " + m.mode().Name
		return m, nil
	case "tab":
		m.cycleFocus()
		return m, nil
	case "f2":
		m.picker.Open(m.cfg.Models, m.modelAlias)
		m.overlay = overlayModel
		return m, nil
	case "ctrl+k":
		return m.startCompaction()
	case "ctrl+n":
		if err := spawn.NewWindow(m.cfg.TerminalTemplate, m.cwd); err != nil {
			m.status = "new window failed: " + err.Error()
		}
		return m, nil
	case "ctrl+g":
		if err := spawn.NewWindow(m.cfg.TerminalTemplate, m.cfg.HomeDir); err != nil {
			m.status = "new window failed: " + err.Error()
		}
		return m, nil
	case "/":
		m.typing = true
		m.input.SetValue("/")
		m.input.CursorEnd()
		return m, m.input.Focus()
	case "t":
		m.typing = true
		return m, m.input.Focus()
	}

	// Number keys pick a choice regardless of focus.
	if len(msg.Runes) == 1 && msg.Runes[0] >= '1' && msg.Runes[0] <= '9' && m.choices.Len() > 0 {
		return m.selectChoice(int(msg.Runes[0] - '1'))
	}

	// Focus-specific navigation.
	switch m.focus {
	case focusTree:
		switch msg.String() {
		case "up", "k":
			m.tree.Move(-1)
		case "down", "j":
			m.tree.Move(1)
		case "right", "l", "enter":
			if path, open := m.tree.Enter(); open {
				return m.openEditor(path)
			}
		case "left", "h":
			m.tree.Collapse()
		}
	case focusChoices:
		switch msg.String() {
		case "up", "k":
			m.choices.Move(-1)
		case "down", "j":
			m.choices.Move(1)
		case "enter":
			if _, ok := m.choices.Selected(); ok {
				return m.selectChoice(m.choicesCursor())
			}
		}
	}
	return m, nil
}

func (m Model) toggleMic() (tea.Model, tea.Cmd) {
	if m.state == stateRecording {
		wav := m.rec.Stop()
		m.state = stateThinking
		m.status = "Transcribing…"
		return m, m.sttCmd(wav)
	}
	m.cancelInflight() // barge-in
	if err := m.rec.Start(); err != nil {
		m.status = "Mic error: " + err.Error()
		m.state = stateIdle
		return m, nil
	}
	m.state = stateRecording
	m.status = "Listening… (space to stop)"
	return m, nil
}

// ---- pipeline ---------------------------------------------------------

func (m Model) onSTT(msg sttDoneMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.status = "STT error: " + msg.err.Error()
		m.sidecarOK = false
		if m.state == stateThinking {
			m.state = stateIdle
		}
		return m, nil
	}
	text := strings.TrimSpace(msg.text)
	if text == "" {
		m.status = "(heard nothing)"
		if m.state == stateThinking {
			m.state = stateIdle
		}
		return m, nil
	}

	// If choices are showing, try to resolve the utterance to one of them.
	if m.choices.Len() > 0 {
		if idx, ok := matchSpokenChoice(text, m.choicesItems()); ok {
			opt, _ := m.choices.At(idx)
			m.addLine(glyphUser, youStyle, text+"  "+glyphChoice+"  "+opt)
			m.appendPending(opt)
			m.choices.Clear()
			m.relayout()
			if m.state == stateRecording {
				return m, nil
			}
			return m.startClaude()
		}
	}

	m.appendPending(text)
	m.addLine(glyphUser, youStyle, text)
	if m.state == stateRecording {
		return m, nil // still talking; accumulate
	}
	return m.startClaude()
}

func (m *Model) startClaude() (tea.Model, tea.Cmd) {
	prompt := m.buildPrompt()
	if strings.TrimSpace(prompt) == "" {
		return *m, nil
	}
	m.cancelInflight()
	m.gen++
	m.ctx, m.cancel = context.WithCancel(context.Background())
	m.liveText = ""
	m.choices.Clear()
	m.relayout()
	m.state = stateThinking
	m.status = "Thinking…"
	m.cl.SetModel(m.modelAlias)
	m.cl.SetPermission(m.mode().Permission)
	return *m, m.streamStartCmd(m.gen, prompt)
}

func (m Model) onStreamEvent(msg streamEventMsg) (tea.Model, tea.Cmd) {
	if msg.gen != m.gen {
		return m, nil // superseded
	}
	if !msg.ok {
		return m, nil // stream closed
	}
	switch msg.ev.Kind {
	case claude.KindText:
		m.liveText += msg.ev.Text
		return m, readStreamCmd(msg.gen, m.currentCh())
	case claude.KindTool:
		m.status = "▸ " + msg.ev.Tool
		return m, readStreamCmd(msg.gen, m.currentCh())
	case claude.KindError:
		m.status = "Claude error: " + msg.ev.Err.Error()
		m.state = stateIdle
		return m, readStreamCmd(msg.gen, m.currentCh())
	case claude.KindResult:
		res := msg.ev.Result
		m.liveText = ""
		if disp := strings.TrimSpace(res.Display); disp != "" {
			m.addLine(glyphClaude, claudeStyle, disp)
		} else {
			m.addLine(glyphClaude, dimStyle, "(spoken)")
		}
		m.stats.Update(res.ContextTokens, res.ContextWindow, res.CostUSD)
		m.pending = ""
		m.attach = nil
		m.choices.Set(res.Choices)
		if m.choices.Len() > 0 {
			m.status = "Choose, speak, or talk."
		}
		m.relayout()
		m.state = stateSpeaking
		spoken := res.Spoken
		if strings.TrimSpace(spoken) == "" {
			m.state = stateIdle
			return m, readStreamCmd(msg.gen, m.currentCh())
		}
		m.status = "Speaking…"
		return m, tea.Batch(m.ttsCmd(msg.gen, spoken), readStreamCmd(msg.gen, m.currentCh()))
	}
	return m, readStreamCmd(msg.gen, m.currentCh())
}

func (m *Model) startCompaction() (tea.Model, tea.Cmd) {
	m.cancelInflight()
	m.gen++
	m.ctx, m.cancel = context.WithCancel(context.Background())
	m.state = stateThinking
	m.status = "Compacting context…"
	gen := m.gen
	ctx := m.ctx
	cl := m.cl
	return *m, func() tea.Msg {
		summary, err := cl.Summarize(ctx)
		return compactDoneMsg{gen: gen, summary: summary, err: err}
	}
}

// ---- choices / editor helpers ----------------------------------------

func (m Model) selectChoice(idx int) (tea.Model, tea.Cmd) {
	opt, ok := m.choices.At(idx)
	if !ok {
		return m, nil
	}
	m.addLine(glyphUser, youStyle, opt)
	m.appendPending(opt)
	m.choices.Clear()
	m.relayout()
	return m.startClaude()
}

func (m Model) submitText() (tea.Model, tea.Cmd) {
	text := strings.TrimSpace(m.input.Value())
	m.typing = false
	m.input.Blur()
	m.input.SetValue("")
	// Slash commands are handled by the app, not sent to Claude.
	if strings.HasPrefix(text, "/") {
		return m.runCommand(text)
	}
	if text == "" && len(m.attach) == 0 {
		return m, nil
	}
	if text != "" {
		m.appendPending(text)
		m.addLine(glyphUser, youStyle, text)
	}
	return m.startClaude()
}

// runCommand dispatches a slash command typed into the message bar.
func (m Model) runCommand(text string) (tea.Model, tea.Cmd) {
	fields := strings.Fields(text)
	cmd := strings.ToLower(fields[0])
	arg := ""
	if len(fields) > 1 {
		arg = strings.ToLower(fields[1])
	}
	switch cmd {
	case "/model":
		if arg == "" {
			m.picker.Open(m.cfg.Models, m.modelAlias)
			m.overlay = overlayModel
			return m, nil
		}
		m.modelAlias = arg
		m.cl.SetModel(arg)
		m.status = "Model → " + arg
	case "/mode":
		if idx := modeIndexByName(arg); idx >= 0 {
			m.modeIdx = idx
		} else {
			m.modeIdx = (m.modeIdx + 1) % len(config.Modes)
		}
		m.cl.SetPermission(m.mode().Permission)
		m.status = "Mode → " + m.mode().Name
	case "/theme":
		if arg == "" {
			m.status = "theme: " + activeThemeName + " · options: " + strings.Join(themeNames(), ", ")
		} else if applyTheme(arg) {
			m.status = "Theme → " + arg
		} else {
			m.status = "unknown theme: " + arg + " (try: " + strings.Join(themeNames(), ", ") + ")"
		}
	case "/compact", "/compress":
		return m.startCompaction()
	case "/new":
		dir := m.cwd
		if arg == "home" {
			dir = m.cfg.HomeDir
		}
		if err := spawn.NewWindow(m.cfg.TerminalTemplate, dir); err != nil {
			m.status = "new window failed: " + err.Error()
		}
	case "/help":
		m.overlay = overlayHelp
	case "/quit", "/exit", "/q":
		m.cancelInflight()
		return m, tea.Quit
	default:
		m.status = "unknown command: " + cmd
	}
	return m, nil
}

func modeIndexByName(name string) int {
	for i, md := range config.Modes {
		if strings.EqualFold(md.Name, name) || strings.EqualFold(md.Permission, name) {
			return i
		}
	}
	return -1
}

func (m Model) openEditor(path string) (tea.Model, tea.Cmd) {
	if err := m.editor.Open(path); err != nil {
		m.status = "open failed: " + err.Error()
		return m, nil
	}
	w := m.width * 9 / 10
	h := m.height * 85 / 100
	m.editor.SetSize(w, h)
	m.overlay = overlayEditor
	return m, nil
}

func (m Model) editorPath() string { return m.editor.path }

func (m *Model) cycleFocus() {
	order := []focusArea{focusConversation, focusTree}
	if m.choices.Len() > 0 {
		order = append(order, focusChoices)
	}
	cur := 0
	for i, f := range order {
		if f == m.focus {
			cur = i
		}
	}
	m.focus = order[(cur+1)%len(order)]
}

func (m *Model) appendPending(text string) {
	if m.pending == "" {
		m.pending = text
	} else {
		m.pending += " " + text
	}
}

func (m *Model) buildPrompt() string {
	p := m.pending
	if len(m.attach) > 0 {
		if p != "" {
			p += "\n\n"
		}
		p += "Attached image file(s) — use your Read tool to view them:\n"
		for _, a := range m.attach {
			p += "- " + a + "\n"
		}
	}
	return p
}

func (m *Model) handlePaste(s string) {
	if p, ok := imagePath(s); ok {
		m.attach = append(m.attach, p)
		m.status = glyphAttach + " attached " + filepath.Base(p)
		return
	}
	var leftover []string
	for _, tok := range strings.Fields(s) {
		if p, ok := imagePath(tok); ok {
			m.attach = append(m.attach, p)
			m.status = glyphAttach + " attached " + filepath.Base(p)
		} else {
			leftover = append(leftover, tok)
		}
	}
	if m.typing && len(leftover) > 0 {
		cur := m.input.Value()
		if cur != "" {
			cur += " "
		}
		m.input.SetValue(cur + strings.Join(leftover, " "))
	}
}

func (m *Model) cancelInflight() {
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	m.gen++
}

func (m *Model) addLine(glyph string, glyphStyle lipgloss.Style, text string) {
	m.transcript = append(m.transcript, convLine{glyph: glyph, glyphStyle: glyphStyle, text: text})
}

func (m Model) choicesItems() []string         { return m.choices.items }
func (m Model) choicesCursor() int             { return m.choices.cursor }
func (m Model) currentCh() <-chan claude.Event { return m.streamCh }

// ---- commands ---------------------------------------------------------

func tickCmd() tea.Cmd {
	return tea.Tick(90*time.Millisecond, func(time.Time) tea.Msg { return tickMsg{} })
}

func (m Model) healthCmd() tea.Cmd {
	base := m.cfg.SidecarURL
	return func() tea.Msg {
		c := &http.Client{Timeout: 1500 * time.Millisecond}
		resp, err := c.Get(base + "/health")
		ok := err == nil && resp != nil && resp.StatusCode == http.StatusOK
		if resp != nil {
			resp.Body.Close()
		}
		return healthMsg{ok: ok}
	}
}

func (m Model) sttCmd(wav []byte) tea.Cmd {
	c := m.stt
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		text, err := c.Transcribe(ctx, wav)
		return sttDoneMsg{text: text, err: err}
	}
}

func (m Model) streamStartCmd(gen int, prompt string) tea.Cmd {
	cl, ctx := m.cl, m.ctx
	return func() tea.Msg {
		ch, err := cl.Stream(ctx, prompt)
		return streamStartedMsg{gen: gen, ch: ch, err: err}
	}
}

func readStreamCmd(gen int, ch <-chan claude.Event) tea.Cmd {
	if ch == nil {
		return nil
	}
	return func() tea.Msg {
		ev, ok := <-ch
		return streamEventMsg{gen: gen, ev: ev, ok: ok}
	}
}

func (m Model) ttsCmd(gen int, text string) tea.Cmd {
	t, ctx := m.tts, m.ctx
	return func() tea.Msg {
		wav, err := t.Synthesize(ctx, text)
		return ttsDoneMsg{gen: gen, wav: wav, err: err}
	}
}

func (m Model) playCmd(gen int, wav []byte) tea.Cmd {
	p, ctx := m.player, m.ctx
	return func() tea.Msg {
		err := p.Play(ctx, wav)
		return playDoneMsg{gen: gen, err: err}
	}
}

// ---- mouse ------------------------------------------------------------

func (m Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.overlay != overlayNone {
		return m, nil
	}
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		if msg.X < m.lay.treeW {
			m.tree.Move(-1)
		} else if m.choices.Len() > 0 {
			m.choices.Move(-1)
		}
		return m, nil
	case tea.MouseButtonWheelDown:
		if msg.X < m.lay.treeW {
			m.tree.Move(1)
		} else if m.choices.Len() > 0 {
			m.choices.Move(1)
		}
		return m, nil
	}
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return m, nil
	}
	// Tree click (inside the left bordered box).
	if msg.X >= 1 && msg.X < m.lay.treeW-1 && msg.Y >= 1 && msg.Y <= m.lay.treeContentH {
		m.focus = focusTree
		m.tree.ClickRow(msg.Y - 1)
		if path, open := m.tree.Enter(); open {
			return m.openEditor(path)
		}
		return m, nil
	}
	// Choice click.
	if m.choices.Len() > 0 && msg.X >= m.lay.convX && msg.Y > m.lay.choicesY && msg.Y < m.lay.choicesY+m.lay.choiceRows {
		idx := msg.Y - (m.lay.choicesY + 1)
		return m.selectChoice(idx)
	}
	return m, nil
}

// ---- layout & view ----------------------------------------------------

func (m *Model) relayout() {
	w, h := m.width, m.height
	if w < 24 || h < 10 {
		return
	}
	mainH := h - 1
	treeW := w / 3
	if treeW < 22 {
		treeW = 22
	}
	if treeW > 40 {
		treeW = 40
	}
	if treeW > w-24 {
		treeW = w - 24
	}
	convW := w - treeW

	choiceRows := 0
	if n := m.choices.Len(); n > 0 {
		choiceRows = n + 1
		if choiceRows > h/2 {
			choiceRows = h / 2
		}
	}
	hintY := mainH - 1
	waveY := hintY - waveRows
	choicesY := waveY - choiceRows
	convBoxH := choicesY
	if convBoxH < 3 {
		convBoxH = 3
	}
	convContentH := convBoxH - 2
	if convContentH < 1 {
		convContentH = 1
	}
	convContentW := convW - 2
	if convContentW < 1 {
		convContentW = 1
	}
	treeContentH := mainH - 2
	if treeContentH < 1 {
		treeContentH = 1
	}

	m.lay = layout{
		treeW: treeW, treeContentH: treeContentH,
		convX: treeW, convContentW: convContentW, convContentH: convContentH, convBoxH: convBoxH,
		waveY: waveY, choicesY: choicesY, choiceRows: choiceRows, hintY: hintY, mainH: mainH,
	}
	m.tree.SetSize(treeW-2, treeContentH)
	m.input.Width = convContentW - 4
}

func (m Model) View() string {
	if m.width < 24 || m.height < 10 {
		return "voxRobota — terminal too small"
	}
	switch m.overlay {
	case overlayHelp:
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, helpOverlay(m.width, m.height))
	case overlayModel:
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, m.picker.View())
	case overlayEditor:
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, m.editor.View())
	}

	// Left: file tree.
	treeTitle := subtitleStyle.Render("files")
	if m.focus == focusTree {
		treeTitle = titleStyle.Render("files")
	}
	_ = treeTitle
	leftBox := border(m.focus == focusTree).
		Width(m.lay.treeW - 2).
		Height(m.lay.treeContentH).
		Render(m.tree.View())

	// Right: conversation box.
	convBox := border(m.focus == focusConversation).
		Width(m.lay.convContentW).
		Height(m.lay.convContentH).
		Render(m.conversationView())

	// Waveform strip.
	wave := m.waveView(m.lay.convContentW + 2)

	// Choices.
	choicesView := ""
	if m.lay.choiceRows > 0 {
		choicesView = m.choices.View(m.lay.convContentW)
	}

	// Footer (input / attachments / hint).
	footer := m.footerView(m.lay.convContentW + 2)

	rightParts := []string{convBox}
	if choicesView != "" {
		rightParts = append(rightParts, choicesView)
	}
	rightParts = append(rightParts, wave, footer)
	right := lipgloss.JoinVertical(lipgloss.Left, rightParts...)

	main := lipgloss.JoinHorizontal(lipgloss.Top, leftBox, right)

	status := renderStatusBar(m.width, m.mode().Name, m.modelAlias, m.stats, m.sidecarOK)
	return lipgloss.JoinVertical(lipgloss.Left, main, status)
}

func (m Model) conversationView() string {
	w := m.lay.convContentW - 3 // 2-col glyph gutter + 1 left pad
	var rows []string
	rows = append(rows, titleStyle.Render("voxRobota")+dimStyle.Render("  ·  ")+stateStyle(m.state).Render(stateLabel(m.state)))

	emit := func(glyph string, gs lipgloss.Style, body string) {
		for i, ln := range wrapText(body, w) {
			if i == 0 {
				rows = append(rows, gs.Render(glyph)+"  "+textStyle.Render(ln))
			} else {
				rows = append(rows, "   "+textStyle.Render(ln))
			}
		}
	}
	for _, cl := range m.transcript {
		emit(cl.glyph, cl.glyphStyle, cl.text)
	}
	if m.liveText != "" {
		if live := claude.CleanDisplay(m.liveText, m.cfg.SpokenMarker, m.cfg.ChoicesMarker); strings.TrimSpace(live) != "" {
			emit(glyphClaude, claudeStyle, live)
		}
	}
	// tail
	h := m.lay.convContentH
	if len(rows) > h {
		rows = rows[len(rows)-h:]
	}
	for i := range rows {
		rows[i] = " " + rows[i]
	}
	return strings.Join(rows, "\n")
}

func (m Model) waveView(w int) string {
	bw := w - 2
	switch m.state {
	case stateRecording:
		return gutterLines(renderWaveTall(m.rec.Levels(bw), bw, waveRows, colBlue))
	case stateSpeaking:
		return gutterLines(renderWaveTall(m.player.Levels(bw), bw, waveRows, colPink))
	default:
		return gutterLines(flatWaveTall(bw, waveRows, colDim))
	}
}

func gutterLines(s string) string {
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = " " + lines[i]
	}
	return strings.Join(lines, "\n")
}

func (m Model) footerView(w int) string {
	if m.typing {
		return truncate(m.input.View(), w)
	}
	var parts []string
	if len(m.attach) > 0 {
		names := make([]string, 0, len(m.attach))
		for _, a := range m.attach {
			names = append(names, filepath.Base(a))
		}
		parts = append(parts, youStyle.Render(glyphAttach+" "+strings.Join(names, ", ")))
	}
	if m.pending != "" {
		parts = append(parts, dimStyle.Render("queued: "+m.pending))
	}
	if len(parts) == 0 {
		return truncate(footerHint(), w)
	}
	parts = append(parts, dimStyle.Render(m.status))
	return truncate(strings.Join(parts, "  ·  "), w)
}

func stateStyle(s state) lipgloss.Style {
	switch s {
	case stateRecording:
		return youStyle
	case stateThinking:
		return subtitleStyle
	case stateSpeaking:
		return claudeStyle
	default:
		return dimStyle
	}
}

func stateLabel(s state) string {
	switch s {
	case stateRecording:
		return glyphUser + " recording"
	case stateThinking:
		return "⋯ thinking"
	case stateSpeaking:
		return glyphClaude + " speaking"
	default:
		return glyphIdle + " ready"
	}
}

// ---- attachments ------------------------------------------------------

func imagePath(tok string) (string, bool) {
	tok = strings.TrimSpace(tok)
	tok = strings.Trim(tok, "'\"")
	if strings.HasPrefix(tok, "file://") {
		tok = strings.TrimPrefix(tok, "file://")
		if u, err := url.PathUnescape(tok); err == nil {
			tok = u
		}
	}
	switch strings.ToLower(filepath.Ext(tok)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".svg":
	default:
		return "", false
	}
	if fi, err := os.Stat(tok); err == nil && !fi.IsDir() {
		return tok, true
	}
	return "", false
}
