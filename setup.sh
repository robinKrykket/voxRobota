#!/usr/bin/env bash
# voxclaude setup — installs the toolchain and models this project needs.
#
#   ./setup.sh          # do everything: system deps, Go, sidecar, models
#   ./setup.sh go       # just install Go (into ~/.local/go) if missing
#   ./setup.sh sidecar  # just create the python venv + install deps + models
#   ./setup.sh models   # just download the Kokoro model files
set -euo pipefail
cd "$(dirname "$0")"

GO_VERSION="1.22.5"
KOKORO_BASE="https://github.com/thewh1teagle/kokoro-onnx/releases/download/model-files-v1.0"

info() { printf '\033[1;36m[setup]\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m[setup]\033[0m %s\n' "$*"; }

install_system() {
  info "checking system build deps (gcc, alsa, curl)…"
  local missing=()
  command -v gcc  >/dev/null || missing+=(build-essential)
  command -v curl >/dev/null || missing+=(curl)
  # miniaudio (malgo) dlopens libasound at runtime.
  ldconfig -p 2>/dev/null | grep -q libasound || missing+=(libasound2-dev)
  if [ "${#missing[@]}" -gt 0 ]; then
    warn "need system packages: ${missing[*]}"
    warn "run:  sudo apt-get update && sudo apt-get install -y ${missing[*]}"
    warn "(re-run this script after installing them)"
  else
    info "system deps present."
  fi
}

install_go() {
  if command -v go >/dev/null; then
    info "go already installed: $(go version)"
    return
  fi
  if [ -x "$HOME/.local/go/bin/go" ]; then
    info "go already at ~/.local/go — add it to PATH (see below)."
  else
    info "installing Go $GO_VERSION into ~/.local/go…"
    mkdir -p "$HOME/.local"
    curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" -o /tmp/go.tgz
    rm -rf "$HOME/.local/go"
    tar -C "$HOME/.local" -xzf /tmp/go.tgz
    rm -f /tmp/go.tgz
  fi
  warn "add Go to your PATH (append to ~/.bashrc):"
  warn '    export PATH="$HOME/.local/go/bin:$PATH"'
  export PATH="$HOME/.local/go/bin:$PATH"
}

setup_sidecar() {
  info "creating python venv (.venv)…"
  python3 -m venv .venv
  info "installing python deps…"
  .venv/bin/pip install --upgrade pip >/dev/null
  .venv/bin/pip install -r sidecar/requirements.txt
  download_models
}

download_models() {
  info "downloading Kokoro model files…"
  [ -f kokoro-v1.0.onnx ] || curl -fL "$KOKORO_BASE/kokoro-v1.0.onnx" -o kokoro-v1.0.onnx
  [ -f voices-v1.0.bin ]  || curl -fL "$KOKORO_BASE/voices-v1.0.bin"  -o voices-v1.0.bin
  info "models ready."
}

case "${1:-all}" in
  go)      install_go ;;
  sidecar) setup_sidecar ;;
  models)  download_models ;;
  all)
    install_system
    install_go
    setup_sidecar
    info "done. Next:"
    info "  1) ensure Go is on PATH (see above)"
    info "  2) go mod tidy && make build"
    info "  3) terminal A:  make sidecar     terminal B:  make run"
    ;;
  *) echo "usage: $0 [all|go|sidecar|models]"; exit 1 ;;
esac
