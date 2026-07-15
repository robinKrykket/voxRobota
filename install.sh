#!/usr/bin/env bash
# voxRobota installer for Linux & macOS.
#   - installs build/runtime deps (C compiler, Python, Go, ALSA on Linux)
#   - creates a Python venv + downloads the Kokoro models
#   - builds the voxrobota binary
#   - installs a `voxrobota` launcher on your PATH (auto-starts the sidecar)
#
# Windows users: run install.ps1 in PowerShell instead.
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
APPDIR="${VOX_APPDIR:-$HOME/.local/share/voxrobota}"
BINDIR="${VOX_BINDIR:-$HOME/.local/bin}"
GO_VERSION="1.22.5"
KOKORO_BASE="https://github.com/thewh1teagle/kokoro-onnx/releases/download/model-files-v1.0"

log()  { printf '\033[1;36m[voxRobota]\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m[voxRobota]\033[0m %s\n' "$*"; }
die()  { printf '\033[1;31m[voxRobota]\033[0m %s\n' "$*"; exit 1; }

OS="$(uname -s)"; ARCH="$(uname -m)"
case "$ARCH" in x86_64|amd64) GOARCH=amd64;; arm64|aarch64) GOARCH=arm64;; *) GOARCH=amd64;; esac
case "$OS" in
	Linux)  GOOS=linux;  PLATFORM=linux;;
	Darwin) GOOS=darwin; PLATFORM=mac;;
	*) die "Unsupported OS '$OS'. On Windows use install.ps1.";;
esac

PM=""
if [ "$PLATFORM" = mac ]; then
	command -v brew >/dev/null 2>&1 && PM=brew
else
	for c in apt-get dnf pacman zypper; do command -v "$c" >/dev/null 2>&1 && PM="$c" && break; done
fi

sudo_if() { if [ "$(id -u)" -ne 0 ]; then sudo "$@"; else "$@"; fi; }

install_pkgs() {
	case "$PM" in
		apt-get) sudo_if apt-get update -y && sudo_if apt-get install -y "$@";;
		dnf)     sudo_if dnf install -y "$@";;
		pacman)  sudo_if pacman -Sy --noconfirm "$@";;
		zypper)  sudo_if zypper install -y "$@";;
		brew)    brew install "$@";;
		*)       warn "no known package manager; please install manually: $*"; return 1;;
	esac
}

ensure_system() {
	log "checking system dependencies…"
	if ! command -v cc >/dev/null 2>&1 && ! command -v gcc >/dev/null 2>&1 && ! command -v clang >/dev/null 2>&1; then
		if [ "$PLATFORM" = mac ]; then
			log "installing Xcode command line tools…"; xcode-select --install 2>/dev/null || true
			warn "finish any Xcode CLT dialog that appeared, then re-run this installer."
		else
			case "$PM" in
				apt-get) install_pkgs build-essential;;
				dnf)     install_pkgs gcc gcc-c++ make;;
				pacman)  install_pkgs base-devel;;
				zypper)  install_pkgs gcc gcc-c++ make;;
			esac
		fi
	fi
	command -v curl >/dev/null 2>&1 || install_pkgs curl || true
	if ! command -v python3 >/dev/null 2>&1; then
		if [ "$PLATFORM" = mac ]; then install_pkgs python@3.12 || install_pkgs python
		else case "$PM" in apt-get) install_pkgs python3 python3-venv python3-pip;; *) install_pkgs python3;; esac; fi
	elif [ "$PM" = apt-get ]; then
		# Debian/Ubuntu split venv/pip into separate packages.
		python3 -c 'import venv' 2>/dev/null || install_pkgs python3-venv python3-pip
	fi
	# ALSA is loaded at runtime by miniaudio; install it if the shared lib is absent.
	if [ "$PLATFORM" = linux ] && ! (ldconfig -p 2>/dev/null | grep -q libasound); then
		case "$PM" in
			apt-get) install_pkgs libasound2 libasound2-dev;;
			dnf)     install_pkgs alsa-lib alsa-lib-devel;;
			pacman)  install_pkgs alsa-lib;;
			zypper)  install_pkgs alsa-lib alsa-lib-devel;;
		esac
	fi
}

ensure_go() {
	if command -v go >/dev/null 2>&1; then log "go present: $(go version)"; return; fi
	if [ -x "$HOME/.local/go/bin/go" ]; then export PATH="$HOME/.local/go/bin:$PATH"; log "using go at ~/.local/go"; return; fi
	log "installing Go $GO_VERSION ($GOOS-$GOARCH)…"
	mkdir -p "$HOME/.local"
	curl -fsSL "https://go.dev/dl/go${GO_VERSION}.${GOOS}-${GOARCH}.tar.gz" -o /tmp/voxgo.tgz
	rm -rf "$HOME/.local/go"; tar -C "$HOME/.local" -xzf /tmp/voxgo.tgz; rm -f /tmp/voxgo.tgz
	export PATH="$HOME/.local/go/bin:$PATH"
	add_path_line 'export PATH="$HOME/.local/go/bin:$PATH"'
}

add_path_line() {
	local line="$1"
	for rc in "$HOME/.bashrc" "$HOME/.zshrc" "$HOME/.profile"; do
		[ -e "$rc" ] || continue
		grep -qF "$line" "$rc" 2>/dev/null || printf '\n%s\n' "$line" >> "$rc"
	done
}

ensure_path() {
	case ":$PATH:" in *":$BINDIR:"*) return;; esac
	add_path_line "export PATH=\"$BINDIR:\$PATH\""
	warn "added $BINDIR to PATH — open a new terminal (or 'source' your shell rc) for it to take effect."
}

setup_appdir() {
	log "installing into $APPDIR"
	mkdir -p "$APPDIR/bin" "$BINDIR"
	rm -rf "$APPDIR/sidecar"; cp -R "$HERE/sidecar" "$APPDIR/sidecar"

	log "creating python venv + installing STT/TTS deps…"
	python3 -m venv "$APPDIR/.venv"
	"$APPDIR/.venv/bin/pip" install --upgrade pip >/dev/null
	"$APPDIR/.venv/bin/pip" install -r "$APPDIR/sidecar/requirements.txt"

	log "fetching Kokoro models…"
	for f in kokoro-v1.0.onnx voices-v1.0.bin; do
		if [ -f "$APPDIR/$f" ]; then continue; fi
		if [ -f "$HERE/$f" ]; then cp "$HERE/$f" "$APPDIR/$f"; else curl -fL "$KOKORO_BASE/$f" -o "$APPDIR/$f"; fi
	done

	log "building voxrobota (cgo)…"
	( cd "$HERE" && CGO_ENABLED=1 go build -o "$APPDIR/bin/voxrobota-bin" . )
}

write_launcher() {
	local launcher="$BINDIR/voxrobota"
	cat > "$launcher" <<EOF
#!/usr/bin/env bash
# voxRobota launcher — starts the STT/TTS sidecar if needed, then the app.
APPDIR="$APPDIR"
export VOX_KOKORO_MODEL="\$APPDIR/kokoro-v1.0.onnx"
export VOX_KOKORO_VOICES="\$APPDIR/voices-v1.0.bin"
if command -v curl >/dev/null 2>&1; then
	if ! curl -s -o /dev/null --max-time 1 http://127.0.0.1:8123/health 2>/dev/null; then
		( cd "\$APPDIR" && nohup "\$APPDIR/.venv/bin/python" "\$APPDIR/sidecar/server.py" >"\$APPDIR/sidecar.log" 2>&1 & )
	fi
fi
exec "\$APPDIR/bin/voxrobota-bin" "\$@"
EOF
	chmod +x "$launcher"
	log "launcher installed: $launcher"
}

main() {
	ensure_system
	ensure_go
	setup_appdir
	write_launcher
	ensure_path
	log "done ✓"
	log "open a NEW terminal in any folder and run:  voxrobota"
	warn "first launch downloads the Whisper model (~150MB) and takes ~20s before speech works (watch the sidecar dot go green)."
}
main "$@"
