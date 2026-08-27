#!/usr/bin/env bash
set -euo pipefail
REPO="${TUNNEL_REPO:-migandhi/tunnel}"
VERSION="${1:-latest}"

OS=$(uname -s | tr '[:upper:]' '[:lower:]')       # linux | darwin
case "$OS" in linux|darwin) ;; *) echo "unsupported OS: $OS"; exit 1;; esac
ARCH=$(uname -m)
case "$ARCH" in x86_64|amd64) ARCH=amd64;; aarch64|arm64) ARCH=arm64;; *) echo "unsupported arch: $ARCH"; exit 1;; esac

NAME="tunnel-client-$OS-$ARCH"
if [ "$VERSION" = latest ]; then
  URL="https://github.com/$REPO/releases/latest/download/$NAME"
else
  URL="https://github.com/$REPO/releases/download/$VERSION/$NAME"
fi

DEST=/usr/local/bin/tunnel-client
TMP=$(mktemp)
echo "Downloading $URL"
curl -fSL -o "$TMP" "$URL"
chmod 755 "$TMP"

if [ -w "$(dirname "$DEST")" ]; then
  mv "$TMP" "$DEST"
elif command -v sudo >/dev/null; then
  sudo mv "$TMP" "$DEST"
else
  DEST="$HOME/.local/bin/tunnel-client"
  mkdir -p "$(dirname "$DEST")"
  mv "$TMP" "$DEST"
  echo "NOTE: installed to $DEST — make sure ~/.local/bin is in your PATH"
fi

if [ "$OS" = darwin ]; then xattr -d com.apple.quarantine "$DEST" 2>/dev/null || true; fi
echo "Installed: $("$DEST" version)  ->  $DEST"
echo "Usage: tunnel-client http 8000 --server tun.example.com:7000 --token tk_..."
