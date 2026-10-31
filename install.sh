#!/bin/sh
# install.sh — bootstrap installer for cmam.
#   curl -fsSL https://raw.githubusercontent.com/cmerk2021/cmam/main/install.sh | sh
#
# Detects the architecture, downloads the latest cmam release, verifies its
# SHA-256 checksum, and installs it to ~/.local/bin. Linux only. No sudo.
set -eu

REPO="cmerk2021/cmam"
BIN_DIR="${HOME}/.local/bin"

say() { printf '%s\n' "$*" >&2; }
die() { say "error: $*"; exit 1; }

[ "$(uname -s)" = "Linux" ] || die "cmam only supports Linux (detected $(uname -s))"

case "$(uname -m)" in
  x86_64 | amd64) ARCH=amd64 ;;
  aarch64 | arm64) ARCH=arm64 ;;
  *) die "unsupported architecture: $(uname -m) (cmam supports amd64 and arm64)" ;;
esac

command -v curl >/dev/null 2>&1 || die "curl is required"

# Resolve the latest tag from the releases/latest redirect (no API quota).
LOCATION=$(curl -fsSI -o /dev/null -w '%{redirect_url}' "https://github.com/${REPO}/releases/latest") ||
  die "could not reach github.com"
TAG=${LOCATION##*/}
case "$TAG" in
  v[0-9]*) ;;
  *) die "could not determine the latest release tag (got '${TAG}')" ;;
esac
VERSION=${TAG#v}

ASSET="cmam_${VERSION}_linux_${ARCH}.tar.gz"
BASE="https://github.com/${REPO}/releases/download/${TAG}"

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT INT TERM

say "downloading cmam ${TAG} (${ARCH})..."
curl -fsSL -o "${TMP}/${ASSET}" "${BASE}/${ASSET}" || die "download failed: ${BASE}/${ASSET}"
curl -fsSL -o "${TMP}/checksums.txt" "${BASE}/checksums.txt" || die "download failed: checksums.txt"

say "verifying checksum..."
EXPECTED=$(grep " [*]\{0,1\}${ASSET}\$" "${TMP}/checksums.txt" | awk '{print $1}')
[ -n "$EXPECTED" ] || die "no checksum entry for ${ASSET}; refusing to install"
if command -v sha256sum >/dev/null 2>&1; then
  ACTUAL=$(sha256sum "${TMP}/${ASSET}" | awk '{print $1}')
elif command -v shasum >/dev/null 2>&1; then
  ACTUAL=$(shasum -a 256 "${TMP}/${ASSET}" | awk '{print $1}')
else
  die "sha256sum (or shasum) is required to verify the download"
fi
[ "$EXPECTED" = "$ACTUAL" ] || die "checksum mismatch for ${ASSET} (expected ${EXPECTED}, got ${ACTUAL})"

say "installing to ${BIN_DIR}/cmam..."
mkdir -p "$BIN_DIR"
tar -xzf "${TMP}/${ASSET}" -C "$TMP" cmam
chmod 0755 "${TMP}/cmam"
mv "${TMP}/cmam" "${BIN_DIR}/cmam"

say "cmam ${TAG} installed."

case ":${PATH}:" in
  *":${BIN_DIR}:"*) ;;
  *)
    say ""
    say "note: ${BIN_DIR} is not on your PATH. Add it with:"
    case "${SHELL:-}" in
      */fish) say "  fish_add_path ${BIN_DIR}    # in ~/.config/fish/config.fish" ;;
      */zsh) say "  export PATH=\"${BIN_DIR}:\$PATH\"    # in ~/.zshrc" ;;
      *) say "  export PATH=\"${BIN_DIR}:\$PATH\"    # in ~/.bashrc" ;;
    esac
    ;;
esac

say ""
say "get started:  cmam search"
