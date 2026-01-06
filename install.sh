#!/usr/bin/env sh
set -eu

REPO="dapi/port-selector"
BINARY="port-selector"

# -------- configurable via env --------
VERSION="${VERSION:-latest}"          # e.g. v0.1.0 or latest
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"
USE_SUDO="${USE_SUDO:-auto}"           # auto | always | never
# --------------------------------------

info()  { printf "%s\n" "$*"; }
warn()  { printf "WARN: %s\n" "$*" >&2; }
error() { printf "ERROR: %s\n" "$*" >&2; exit 1; }

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || error "command not found: $1"
}

need_cmd uname
need_cmd curl
need_cmd chmod
need_cmd mkdir
need_cmd mktemp

OS="$(uname -s)"
ARCH="$(uname -m)"

case "$OS" in
  Linux)  OS="linux" ;;
  Darwin) OS="darwin" ;;
  *) error "unsupported OS: $OS" ;;
esac

case "$ARCH" in
  x86_64|amd64) ARCH="amd64" ;;
  arm64|aarch64) ARCH="arm64" ;;
  *) error "unsupported arch: $ARCH" ;;
esac

ASSET="${BINARY}-${OS}-${ARCH}"

if [ "$VERSION" = "latest" ]; then
  URL="https://github.com/${REPO}/releases/latest/download/${ASSET}"
else
  URL="https://github.com/${REPO}/releases/download/${VERSION}/${ASSET}"
fi

info "Installing ${BINARY}"
info "  OS/ARCH: ${OS}/${ARCH}"
info "  Version: ${VERSION}"
info "  Asset:   ${ASSET}"
info "  URL:     ${URL}"
info "  Target:  ${INSTALL_DIR}/${BINARY}"

TMPDIR="$(mktemp -d)"
cleanup() { rm -rf "$TMPDIR"; }
trap cleanup EXIT

TMPBIN="${TMPDIR}/${BINARY}"

info "Downloading..."
curl -fL "$URL" -o "$TMPBIN" || error "download failed"

chmod +x "$TMPBIN"

# decide sudo usage
do_sudo="false"
if [ "$USE_SUDO" = "always" ]; then
  do_sudo="true"
elif [ "$USE_SUDO" = "never" ]; then
  do_sudo="false"
else
  # auto
  if ! mkdir -p "$INSTALL_DIR" 2>/dev/null; then
    do_sudo="true"
  fi
fi

if [ "$do_sudo" = "true" ]; then
  info "Using sudo to install into ${INSTALL_DIR}"
  sudo mkdir -p "$INSTALL_DIR"
  sudo mv "$TMPBIN" "${INSTALL_DIR}/${BINARY}"
else
  mkdir -p "$INSTALL_DIR"
  mv "$TMPBIN" "${INSTALL_DIR}/${BINARY}"
fi

info "Installed ${BINARY} successfully."

case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *)
    warn "${INSTALL_DIR} is not in PATH"
    warn "Add it to your shell config, e.g.:"
    warn "  export PATH=\"${INSTALL_DIR}:\$PATH\""
    ;;
esac

info "Run '${BINARY} --help' to get started."
