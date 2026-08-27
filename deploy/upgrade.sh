#!/usr/bin/env bash
set -euo pipefail
REPO="${TUNNEL_REPO:-migandhi/tunnel}"
VERSION="${1:-latest}"
BIN=/usr/local/bin/tunnel-server

[ "$(id -u)" -eq 0 ] || { echo "run as root (sudo bash upgrade.sh)"; exit 1; }
ARCH=$(uname -m); case "$ARCH" in x86_64) GOARCH=amd64;; aarch64) GOARCH=arm64;; *) echo "unsupported architecture"; exit 1;; esac

echo "==> Backing up binary and database"
cp "$BIN" "$BIN.bak"
if command -v sqlite3 >/dev/null && [ -f /var/lib/tunnel/tunnel.db ]; then
  sqlite3 /var/lib/tunnel/tunnel.db ".backup /var/lib/tunnel/tunnel.db.bak"
elif [ -f /var/lib/tunnel/tunnel.db ]; then
  cp /var/lib/tunnel/tunnel.db /var/lib/tunnel/tunnel.db.bak
fi

if [ "$VERSION" = latest ]; then
  URL="https://github.com/$REPO/releases/latest/download/tunnel-server-linux-$GOARCH"
else
  URL="https://github.com/$REPO/releases/download/$VERSION/tunnel-server-linux-$GOARCH"
fi

echo "==> Downloading $URL"
curl -fSL -o "$BIN.new" "$URL"
chmod 755 "$BIN.new"
mv "$BIN.new" "$BIN"
systemctl restart tunnel-server
sleep 2

if systemctl is-active --quiet tunnel-server; then
  echo "==> Upgrade OK: $("$BIN" version)"
else
  echo "!! Upgrade FAILED. Roll back with:"
  echo "     mv $BIN.bak $BIN"
  echo "     systemctl restart tunnel-server"
  exit 1
fi
