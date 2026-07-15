# voxRobota

A hands-free, neon voice IDE on top of **Claude Code**. Hold a key and talk; your speech is
transcribed locally with Whisper, sent to Claude Code, and Claude's short summary is spoken
back with Kokoro TTS — inside a full terminal UI with a live waveform, a file tree, a built-in
editor, mode/model switching, a token meter, switchable neon themes, and clickable/spoken
response choices.

```
 [space] ─ talk ─▶ whisper ─▶ claude ─▶ spoken summary ─▶ kokoro ─▶ 🔊
   ▲                                                                 │
   └──────── tap space again to interrupt or add to your message ─────┘
```

The spoken summary is **heard, not printed** — the transcript shows only Claude's actual work.

---

## Prerequisites

- **Claude Code** installed and working (`claude --version`). voxRobota drives it; it does not
  install it.
- A working **microphone and speakers** (the app needs an audio device).

Everything else — a C compiler, Go, Python, the speech models — is installed for you.

## Install

Clone/copy this folder onto the machine, then run the installer for your OS.

**Linux / macOS**
```bash
cd voxRobota
./install.sh
```

**Windows (PowerShell)**
```powershell
cd voxRobota
powershell -ExecutionPolicy Bypass -File .\install.ps1
```

The installer will:
1. install a C compiler, Go, and Python if missing,
2. create a Python virtual environment with the speech engines,
3. download the Kokoro voice models (~340 MB),
4. build the `voxrobota` binary,
5. put a **`voxrobota`** command on your PATH.

See **[INSTALL.md](INSTALL.md)** for per-OS details and troubleshooting (the Windows C-compiler
step is the most common snag).

## Run

Open a **new** terminal (so the PATH change takes effect), `cd` into any project folder, and:

```bash
voxrobota
```

That's it — the launcher starts the speech engine in the background and opens the UI rooted at
your current folder. On first launch it downloads the Whisper model (~150 MB) and takes ~20s
before speech works; the **sidecar dot** in the status bar turns green when it's ready.

---

## Using it

**Talk to it.** Press `Space` to start recording, `Space` again to stop and send. Claude works,
then speaks a short summary and tells you what it needs next.

**Interrupt / pile on.** Tap `Space` again while Claude is thinking or talking to cancel the
current turn and start a new recording. Anything you'd already said is kept and **combined** —
so "add a test" → (tap) → "for the parser" is sent as one message.

**Type instead.** Press `t` to type a message, `Enter` to send.

**Commands.** Press `/` to open the command bar:

| Command | Does |
|---|---|
| `/model [name]` | switch model, or open the picker with no argument |
| `/mode [plan\|auto\|manual]` | set permission mode (or cycle) |
| `/theme [vice\|berserk]` | switch theme |
| `/compact` | compact the conversation to free up context |
| `/new [home]` | open a new voxRobota window (here, or at home) |
| `/help` | show the shortcuts overlay |
| `/quit` | exit |

**Answer choices.** When Claude offers options they appear as a numbered list — pick one by
pressing its **number**, with **↑/↓ + Enter**, by **clicking** it, or by **voice** (say the
number, say the option, or just say something else to send a fresh message).

**Attach an image.** Drag an image file onto the window; it's attached to your next message and
Claude views it. (Works in terminals that paste the file path on drop.)

**Browse & edit files.** `Tab` moves focus to the file tree; arrows navigate, `Enter` opens a
file in the built-in editor (`i` to edit, `Esc` to stop editing, `Ctrl+S` to save & close).

## Shortcuts

Bindings are identical on every OS; the matrix below notes the few platform quirks.

| Action | Key |
|---|---|
| Push-to-talk (start / stop / interrupt) | `Space` |
| Cycle mode `PLAN → AUTO → MANUAL` | `Shift+Tab` |
| Cycle focus (conversation ↔ tree ↔ choices) | `Tab` |
| Pick a numbered choice | `1`–`9` |
| Navigate focused panel | `↑ ↓ ← →` |
| Select / open file / expand dir | `Enter` |
| Type a message | `t` |
| Command bar | `/` |
| Model picker | `F2` |
| Compact context now | `Ctrl+K` |
| New window — current folder / home | `Ctrl+N` / `Ctrl+G` |
| Attach image | *drag file onto window* |
| Editor: insert / normal / save & close | `i` / `Esc` / `Ctrl+S` |
| Help overlay | `?` |
| Quit | `q` (when idle) / `Ctrl+C` |

### Per-OS notes

| | Linux | macOS | Windows |
|---|---|---|---|
| Modifier the app sees | `Ctrl` | `Ctrl` (not ⌘) | `Ctrl` |
| `F2` model picker | works | may need `Fn+F2`, or use `/model` | works |
| New-window spawner | `gnome-terminal` | Terminal.app (approve automation once) | `cmd` / Windows Terminal |
| Recommended terminal | any modern | Terminal.app / iTerm2 | **Windows Terminal** (not legacy console) |
| Mouse (click / scroll) | yes | yes | Windows Terminal only |
| Image drag-drop | VTE terminals | Terminal.app / iTerm2 | Windows Terminal |

Override the new-window command anywhere with `VOX_TERMINAL` (space-separated; `{dir}` and
`{bin}` are substituted), e.g. `VOX_TERMINAL="kitty --directory {dir} {bin}"`.

## Themes

- **vice** — Miami Vice: lavender text, neon pink/blue/purple. Your messages are **purple**;
  mic waveform blue, Claude's pink.
- **berserk** — black / red / bone-white: your messages and borders **red**, Claude and the
  selection highlighter **white**; mic waveform red, Claude's white.

Switch live: `/theme vice` · `/theme berserk`.

## Configuration (environment variables)

| Var | Default | Meaning |
|---|---|---|
| `VOX_MODEL` | `opus` | starting model alias |
| `VOX_VOICE` | `af_heart` | Kokoro voice |
| `VOX_LEADIN_MS` | `300` | silence padded before speech to prevent clipped starts |
| `VOX_COMPACT_AT` | `0.80` | context-fill fraction that auto-compacts |
| `VOX_WHISPER_MODEL` | `base.en` | Whisper model (`small.en`, `medium.en`, …) |
| `VOX_WHISPER_DEVICE` | `cpu` | `cuda` on an NVIDIA GPU |
| `VOX_TERMINAL` | per-OS | new-window command template |
| `VOX_SIDECAR_URL` | `http://127.0.0.1:8123` | speech sidecar address |
| `VOX_CLAUDE_BIN` | `claude` | Claude Code executable |

## How it's built

voxRobota is a **front end** to Claude Code — it doesn't fork or modify it. Speech runs in a
small local Python sidecar so the ML stack stays out of the Go build.

| Piece | Tech |
|---|---|
| TUI + orchestration | Go + Bubble Tea (`internal/tui`) |
| Audio capture/playback | `malgo` / miniaudio (`internal/audio`) |
| The brain | `claude -p --output-format stream-json` (`internal/claude`) |
| Speech-to-text | `faster-whisper` (Python sidecar) |
| Text-to-speech | `kokoro-onnx` (Python sidecar) |

Dev build from source: `./setup.sh`, then `make sidecar` (one terminal) and `make run` (another).

## Caveats

- **Compaction is emulated** (summarize → new session); headless Claude has no `/compact`.
- **Choices** rely on Claude ending replies with a `### Choices` list; the parser is tolerant
  and simply shows none when absent.
- **Image drag-drop** depends on the terminal pasting a file path on drop.
- The stream-json schema is parsed from real `claude` output; if it changes, update
  `internal/claude/client.go`.
