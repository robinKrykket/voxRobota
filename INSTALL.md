# Installing voxRobota

voxRobota installs in one command per OS. The installer checks for and installs
everything it needs **except Claude Code itself** (install that separately and
make sure `claude` is on your PATH), then puts a `voxrobota` command on your
PATH so you can `cd` into any folder and run `voxrobota` to launch.

What the installer sets up:

- a **C compiler** (needed to build the audio layer — cgo/miniaudio)
- **Go** (to build the app) and **Python 3** (for the speech sidecar)
- a Python **venv** with `faster-whisper` + `kokoro-onnx`
- the **Kokoro voice models** (~340 MB)
- the **`voxrobota` launcher** on your PATH, which **auto-starts the speech
  sidecar** in the background so a single command "just works"

> **Prerequisite:** Claude Code must already be installed and working
> (`claude --version`). voxRobota drives it; it does not install it.

---

## Linux

```bash
cd voxRobota
./install.sh
```

- Uses your package manager (`apt`, `dnf`, `pacman`, or `zypper`) to install
  `gcc`, Python, and ALSA — you'll be asked for `sudo` if something is missing.
- Installs Go into `~/.local/go` if you don't already have it.
- Launcher goes to `~/.local/bin/voxrobota` (added to PATH via your shell rc).

Open a **new** terminal, then: `voxrobota`

## macOS

```bash
cd voxRobota
./install.sh
```

- Triggers **Xcode Command Line Tools** install (`clang`) if needed — finish the
  dialog, then re-run `./install.sh`.
- Uses **Homebrew** for Python/Go if present (install Homebrew first for the
  smoothest path: <https://brew.sh>).
- The "new window" shortcut uses **Terminal.app** via AppleScript; the first
  time, macOS will ask you to allow automation — approve it.

Open a **new** terminal, then: `voxrobota`

## Windows

```powershell
cd voxRobota
powershell -ExecutionPolicy Bypass -File .\install.ps1
```

- Installs **Go**, **Python**, and **mingw-w64** (the C compiler) via `winget`
  (or `choco` if present). Installing the C compiler is the most failure-prone
  step — see Troubleshooting.
- Launcher goes to `%LOCALAPPDATA%\voxRobota\bin\voxrobota.cmd` and that folder
  is added to your user PATH.
- **Use Windows Terminal** (not the legacy console) for colors, glyphs, and
  mouse support.

Open a **new** terminal, then: `voxrobota`

---

## Shortcuts

The full keyboard/mouse reference and the per-OS behavior matrix (modifier keys,
`F2`, the new-window spawner, mouse, drag-drop) live in the main
**[README.md → Shortcuts](README.md#shortcuts)**.

---

## Configuration (environment variables)

| Var | Default | Meaning |
|---|---|---|
| `VOX_MODEL` | `opus` | starting model alias |
| `VOX_VOICE` | `af_heart` | Kokoro voice |
| `VOX_LEADIN_MS` | `300` | silence padded before speech to stop clipped starts |
| `VOX_COMPACT_AT` | `0.80` | context-fill fraction that auto-compacts |
| `VOX_WHISPER_MODEL` | `base.en` | Whisper model (`small.en`, `medium.en`, …) |
| `VOX_WHISPER_DEVICE` | `cpu` | `cuda` on an NVIDIA GPU |
| `VOX_TERMINAL` | per-OS | new-window command template |
| `VOX_SIDECAR_URL` | `http://127.0.0.1:8123` | speech sidecar address |

Switch themes at runtime with `/theme vice` or `/theme berserk`.

---

## Troubleshooting

- **First launch has no speech for ~20s / sidecar dot is pink.** Normal — the
  sidecar is loading Whisper (downloaded once, ~150 MB) and Kokoro. It turns
  green when ready.
- **Voice starts mid-sentence.** Increase the lead-in: `VOX_LEADIN_MS=500`.
- **Windows: `gcc` not found / build fails.** Install mingw-w64 and ensure it's
  on PATH (`winget install MSYS2.MSYS2` then `pacman -S mingw-w64-x86_64-gcc`, or
  `choco install mingw`), open a new terminal, and re-run `install.ps1`. This is
  the #1 Windows snag because Go ships no C compiler.
- **macOS: build fails with "xcrun: error".** Run `xcode-select --install`,
  finish the dialog, then re-run `./install.sh`.
- **`voxrobota: command not found`.** Open a **new** terminal (PATH changes only
  apply to new shells), or `source ~/.bashrc` / `~/.zshrc`.
- **No audio device.** Ensure your OS sees a mic and speakers; on headless Linux
  there's no audio device and the app can't start.
