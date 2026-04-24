#!/bin/bash
# Savannaa Cloud CLI installer — sws
#
# Installs the latest v* release from github.com/savannaacloud/sws.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/savannaacloud/sws/main/install.sh | sh
#
# Environment overrides:
#   SWS_VERSION       pin a specific version (default: latest)
#   SWS_INSTALL_DIR   target directory (default: /usr/local/bin)
set -e

REPO="savannaacloud/sws"
BINARY="sws"
INSTALL_DIR="${SWS_INSTALL_DIR:-/usr/local/bin}"

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) echo "unsupported arch: $ARCH" >&2; exit 1 ;;
esac

if [ -z "${SWS_VERSION:-}" ]; then
  SWS_VERSION=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" \
    | grep -oE '"tag_name":[[:space:]]*"[^"]+"' \
    | head -1 | sed -E 's/.*"([^"]+)".*/\1/')
fi

if [ -z "$SWS_VERSION" ]; then
  echo "Could not resolve latest release — tag v0.0.0 a release first, or set SWS_VERSION." >&2
  exit 1
fi

URL="https://github.com/$REPO/releases/download/$SWS_VERSION/${BINARY}-${OS}-${ARCH}"
echo "Downloading $URL"

TMP=$(mktemp)
curl -fsSL "$URL" -o "$TMP"
chmod +x "$TMP"

if [ -w "$INSTALL_DIR" ]; then
  mv "$TMP" "$INSTALL_DIR/$BINARY"
else
  sudo mv "$TMP" "$INSTALL_DIR/$BINARY"
fi

echo
echo "Installed $BINARY $SWS_VERSION to $INSTALL_DIR/$BINARY"
echo "Next: sws login"
