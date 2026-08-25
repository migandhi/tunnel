#!/usr/bin/env bash
set -euo pipefail

REPO="${TUNNEL_REPO:-migandhi/tunnel-software}"
VERSION="${TUNNEL_VERSION:-latest}"
BIN=/usr/local/bin/tunnel-server
ENVF=/etc/tunnel/server.env
UNIT=/etc/systemd/system/tunnel-server.service

say(){ echo -e "\e[1;32m==>\e[0m $*"; }
warn(){ echo -e "\e[1;33m!!\e[0m $*"; }
die(){ echo -e "\e[1;31mERROR:\e[0m $*" >&2; exit 1; }

[ "$(id -u)" -eq 0 ] || die "run as root (sudo bash install.sh)"
ARCH=$(uname -m)
case "$ARCH" in x86_64) GOARCH=amd64;; aarch64) GOARCH=arm64;; *) die "unsupported architecture: $ARCH";; esac

apt-get update -qq
apt-get install -y -qq curl ca-certificates >/dev/null

id tunnel &>/dev/null || useradd --system --home /var/lib/tunnel --shell /usr/sbin/nologin tunnel
mkdir -p /var/lib/tunnel /etc/tunnel
chown tunnel:tunnel /var/lib/tunnel
chmod 750 /var/lib/tunnel

if [ -f ./tunnel-server ]; then
  install -m 755 ./tunnel-server "$BIN"
else
  if [ "$VERSION" = latest ]; then
    URL="https://github.com/$REPO/releases/latest/download/tunnel-server-linux-$GOARCH"
  else
    URL="https://github.com/$REPO/releases/download/$VERSION/tunnel-server-linux-$GOARCH"
  fi
  curl -fSL -o "$BIN" "$URL" || die "download failed: $URL"
  chmod 755 "$BIN"
fi

if [ ! -f "$ENVF" ]; then
  read -rp "Base tunnel domain (e.g. tun.example.com): " DOMAIN
  [ -n "$DOMAIN" ] || die "domain required"
  read -rp "ACME/Let's Encrypt email: " EMAIL
  read -rp "Admin username [admin]: " AUSER; AUSER=${AUSER:-admin}
  echo "Choose an admin password (minimum 12 characters)."
  HASH=$("$BIN" hash-password)
  umask 077
  cat > "$ENVF" <<EOF
TUNNEL_DOMAIN=$DOMAIN
TUNNEL_ACME_EMAIL=$EMAIL
TUNNEL_ADMIN_USER=$AUSER
TUNNEL_ADMIN_PASS_HASH=$HASH
TUNNEL_DATA_DIR=/var/lib/tunnel
TUNNEL_TLS_MODE=auto
TUNNEL_TCP_PORT_MIN=20000
TUNNEL_TCP_PORT_MAX=20249
EOF
  chown root:tunnel "$ENVF"
  chmod 640 "$ENVF"
else
  say "Existing configuration kept: $ENVF"
  DOMAIN=$(grep '^TUNNEL_DOMAIN=' "$ENVF" | cut -d= -f2-)
fi

MYIP=$(curl -fs4 https://api.ipify.org || true)
DNSIP=$(getent hosts "$DOMAIN" | awk '{print $1}' | head -1 || true)
if [ -n "$MYIP" ] && [ "$DNSIP" = "$MYIP" ]; then
  say "DNS OK: $DOMAIN -> $MYIP"
else
  warn "DNS check: $DOMAIN -> '${DNSIP:-nothing}', VPS -> '${MYIP:-unknown}'"
  warn "Create A records for $DOMAIN and *.$DOMAIN pointing to this VPS."
fi

install -m 644 ./deploy/systemd/tunnel-server.service "$UNIT" 2>/dev/null || curl -fsSL -o "$UNIT" "https://raw.githubusercontent.com/$REPO/main/deploy/systemd/tunnel-server.service"

systemctl daemon-reload
systemctl enable tunnel-server >/dev/null

if command -v ufw >/dev/null && ufw status | grep -q "Status: active"; then
  echo "Required TCP ports: 22, 80, 443, 7000, 20000-20249"
  read -rp "Add tunnel UFW rules now? [y/N] " OK
  if [[ "${OK,,}" == y* ]]; then
    ufw allow 80/tcp; ufw allow 443/tcp; ufw allow 7000/tcp; ufw allow 20000:20249/tcp
  fi
else
  warn "Open 80, 443, 7000 and 20000-20249 TCP in your firewall / cloud firewall."
fi

systemctl restart tunnel-server
sleep 2
systemctl --no-pager --lines=8 status tunnel-server || true

cat <<EOF
Installed.

Admin UI is localhost-only:
  ssh -L 9800:127.0.0.1:9800 root@$DOMAIN
Then open:
  http://127.0.0.1:9800

Config: /etc/tunnel/server.env
Data:   /var/lib/tunnel
Logs:   journalctl -u tunnel-server -f
EOF
