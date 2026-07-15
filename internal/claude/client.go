package claude

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"sync"
)

// Client drives Claude Code in headless streaming mode. Each Stream runs
// `claude -p ... --output-format stream-json` and emits Events as they
// arrive. Session continuity is kept via --resume.
type Client struct {
	Bin           string
	SystemPrompt  string
	SpokenMarker  string
	ChoicesMarker string

	mu         sync.Mutex
	model      string // alias passed to --model
	permission string // value passed to --permission-mode
	sessionID  string
	seed       string // one-shot context prepended after a compaction reset
}

func (c *Client) SetModel(alias string)   { c.mu.Lock(); c.model = alias; c.mu.Unlock() }
func (c *Client) SetPermission(p string)  { c.mu.Lock(); c.permission = p; c.mu.Unlock() }
func (c *Client) SessionID() string       { c.mu.Lock(); defer c.mu.Unlock(); return c.sessionID }
func (c *Client) ResetSession()           { c.mu.Lock(); c.sessionID = ""; c.mu.Unlock() }

// ResetWithSummary drops the session and arranges for the given summary to
// seed the next turn (our emulation of /compact).
func (c *Client) ResetWithSummary(summary string) {
	c.mu.Lock()
	c.sessionID = ""
	c.seed = summary
	c.mu.Unlock()
}

// ---- events -----------------------------------------------------------

type EventKind int

const (
	KindText   EventKind = iota // Text holds a streamed delta
	KindTool                    // Tool holds the tool name Claude just started
	KindResult                  // Result holds the finished turn
	KindError                   // Err holds a fatal error
)

type Event struct {
	Kind   EventKind
	Text   string
	Tool   string
	Result *Result
	Err    error
}

// Result is a finished Claude turn.
type Result struct {
	Full          string // raw reply text (all sections)
	Display       string // Full with the Spoken + Choices sections removed
	Spoken        string
	Choices       []string
	SessionID     string
	ContextTokens int // input + cache_read + cache_creation on the last turn
	ContextWindow int
	CostUSD       float64
}

// ---- wire types (matching real stream-json output) --------------------

type wireLine struct {
	Type         string                `json:"type"`
	Subtype      string                `json:"subtype"`
	Event        *wireStreamEvent      `json:"event"`
	Result       string                `json:"result"`
	SessionID    string                `json:"session_id"`
	IsError      bool                  `json:"is_error"`
	TotalCostUSD float64               `json:"total_cost_usd"`
	Usage        *wireUsage            `json:"usage"`
	ModelUsage   map[string]wireModelU `json:"modelUsage"`
}

type wireStreamEvent struct {
	Type         string `json:"type"`
	ContentBlock *struct {
		Type string `json:"type"`
		Name string `json:"name"`
	} `json:"content_block"`
	Delta *struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"delta"`
}

type wireUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}

type wireModelU struct {
	ContextWindow int `json:"contextWindow"`
}

// ---- streaming --------------------------------------------------------

// Stream starts a turn and returns a channel of Events, closed when the
// turn ends. Cancelling ctx kills the claude process (used for barge-in and
// coalescing). The channel always receives exactly one terminal event
// (KindResult or KindError) unless ctx was cancelled first.
func (c *Client) Stream(ctx context.Context, prompt string) (<-chan Event, error) {
	cmd := exec.CommandContext(ctx, c.Bin, c.args(prompt)...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	ch := make(chan Event, 128)
	go func() {
		defer close(ch)
		res := Result{}
		haveResult := false

		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 1024*1024), 32*1024*1024) // tolerate big JSON lines
		for sc.Scan() {
			var ln wireLine
			if err := json.Unmarshal(sc.Bytes(), &ln); err != nil {
				continue // ignore anything that isn't a JSON event line
			}
			switch ln.Type {
			case "stream_event":
				if ln.Event == nil {
					continue
				}
				switch ln.Event.Type {
				case "content_block_delta":
					if ln.Event.Delta != nil && ln.Event.Delta.Type == "text_delta" && ln.Event.Delta.Text != "" {
						send(ctx, ch, Event{Kind: KindText, Text: ln.Event.Delta.Text})
					}
				case "content_block_start":
					if ln.Event.ContentBlock != nil && ln.Event.ContentBlock.Type == "tool_use" {
						send(ctx, ch, Event{Kind: KindTool, Tool: ln.Event.ContentBlock.Name})
					}
				}
			case "result":
				res.Full = ln.Result
				res.SessionID = ln.SessionID
				res.CostUSD = ln.TotalCostUSD
				if ln.Usage != nil {
					res.ContextTokens = ln.Usage.InputTokens + ln.Usage.CacheReadInputTokens + ln.Usage.CacheCreationInputTokens
				}
				res.ContextWindow = maxContextWindow(ln.ModelUsage)
				haveResult = true
			}
		}

		werr := cmd.Wait()
		if ctx.Err() != nil {
			return // cancelled — swallow, no terminal event
		}
		if !haveResult {
			send(ctx, ch, Event{Kind: KindError, Err: fmt.Errorf("claude produced no result: %v: %s", werr, strings.TrimSpace(stderr.String()))})
			return
		}

		if res.SessionID != "" {
			c.mu.Lock()
			c.sessionID = res.SessionID
			c.mu.Unlock()
		}
		res.Spoken = c.extractSpoken(res.Full)
		res.Choices = c.extractChoices(res.Full)
		res.Display = CleanDisplay(res.Full, c.SpokenMarker, c.ChoicesMarker)
		send(ctx, ch, Event{Kind: KindResult, Result: &res})
	}()

	return ch, nil
}

// Summarize asks the current session for a compact summary, draining the
// stream. Used by emulated compaction.
func (c *Client) Summarize(ctx context.Context) (string, error) {
	ch, err := c.Stream(ctx, "Summarize our entire conversation so far as plain text: the "+
		"overall task, key decisions made, the current state, and the next steps. Be "+
		"concise but complete enough to continue the work from this summary alone.")
	if err != nil {
		return "", err
	}
	for ev := range ch {
		switch ev.Kind {
		case KindResult:
			return strings.TrimSpace(ev.Result.Full), nil
		case KindError:
			return "", ev.Err
		}
	}
	return "", ctx.Err()
}

func (c *Client) args(prompt string) []string {
	c.mu.Lock()
	model, perm, session, seed := c.model, c.permission, c.sessionID, c.seed
	c.seed = "" // consumed once
	c.mu.Unlock()

	if seed != "" {
		prompt = "Context from our earlier session (it was summarized to save space):\n\n" +
			seed + "\n\n---\n\nContinuing, the user says:\n\n" + prompt
	}

	a := []string{"-p", prompt,
		"--output-format", "stream-json",
		"--verbose",
		"--include-partial-messages",
	}
	if c.SystemPrompt != "" {
		a = append(a, "--append-system-prompt", c.SystemPrompt)
	}
	if model != "" {
		a = append(a, "--model", model)
	}
	if perm != "" {
		a = append(a, "--permission-mode", perm)
	}
	if session != "" {
		a = append(a, "--resume", session)
	}
	return a
}

// ---- reply parsing ----------------------------------------------------

var numberedItem = regexp.MustCompile(`^\s*\d+[.)]\s+(.*\S)\s*$`)

// extractSpoken returns the prose after the Spoken marker, trimmed to that
// section. Falls back to a tail of the reply if the marker is absent.
func (c *Client) extractSpoken(full string) string {
	if s, ok := section(full, c.SpokenMarker); ok && strings.TrimSpace(s) != "" {
		return strings.TrimSpace(s)
	}
	s := strings.TrimSpace(full)
	if len(s) > 400 {
		s = "…" + s[len(s)-400:]
	}
	return s
}

// CleanDisplay returns text with the Spoken and Choices sections removed, so
// the spoken summary is heard (not printed) and choices show only as the
// interactive list. Any other headings/content are preserved. Works on
// partial (streaming) text too.
func CleanDisplay(full, spokenMarker, choicesMarker string) string {
	full = stripSection(full, spokenMarker)
	full = stripSection(full, choicesMarker)
	return strings.TrimRight(full, " \t\n")
}

// stripSection removes `marker` and its body (up to the next "### " heading or
// end of string), keeping any later headings intact.
func stripSection(full, marker string) string {
	i := strings.Index(full, marker)
	if i < 0 {
		return full
	}
	rest := full[i+len(marker):]
	tail := ""
	if j := strings.Index(rest, "\n### "); j >= 0 {
		tail = rest[j:]
	}
	return strings.TrimRight(full[:i], " \t\n") + tail
}

// extractChoices parses the numbered list under the Choices marker.
func (c *Client) extractChoices(full string) []string {
	body, ok := section(full, c.ChoicesMarker)
	if !ok {
		return nil
	}
	var out []string
	for _, line := range strings.Split(body, "\n") {
		if m := numberedItem.FindStringSubmatch(line); m != nil {
			out = append(out, strings.TrimSpace(m[1]))
		}
	}
	return out
}

// section returns the text after `marker` up to the next markdown heading
// ("### ...") or end of string.
func section(full, marker string) (string, bool) {
	i := strings.Index(full, marker)
	if i < 0 {
		return "", false
	}
	rest := full[i+len(marker):]
	if j := strings.Index(rest, "\n### "); j >= 0 {
		rest = rest[:j]
	}
	return rest, true
}

func maxContextWindow(mu map[string]wireModelU) int {
	best := 0
	for _, v := range mu {
		if v.ContextWindow > best {
			best = v.ContextWindow
		}
	}
	return best
}

func send(ctx context.Context, ch chan<- Event, ev Event) {
	select {
	case ch <- ev:
	case <-ctx.Done():
	}
}
