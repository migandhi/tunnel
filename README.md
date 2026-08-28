# Tunnel

**A self-hosted ngrok alternative that runs on a $6/month VPS.**

Expose a web app or TCP service running on your laptop (behind NAT, CGNAT, or
a firewall) to the public internet through your own domain — with automatic
HTTPS, user accounts, bandwidth limits, and an admin dashboard.

```
https://myapp.tun.example.com  ──►  your laptop's localhost:8000
```

- **Server:** one binary, one systemd service, Ubuntu VPS
- **Client:** one binary — Windows, macOS, Linux (amd64 + arm64)
- **No inbound ports needed on the client machine** — it dials out
- **No Docker, no database server, no dependencies** — SQLite built in

> **Status:** Production-oriented, tested on Ubuntu 24.04 with a 1 GB
> DigitalOcean droplet. Read [Limitations](#limitations) before exposing
> anything sensitive.

---

## Table of Contents

- [How It Works](#how-it-works)
- [Features](#features)
- [Quick Start (10 minutes)](#quick-start-10-minutes)
- [Step 1 — DNS Records](#step-1--dns-records)
- [Step 2 — Install the Server](#step-2--install-the-server)
- [Step 3 — Open the Admin Dashboard](#step-3--open-the-admin-dashboard)
- [Step 4 — Install the Client](#step-4--install-the-client)
- [Step 5 — Start a Tunnel](#step-5--start-a-tunnel)
- [TCP Tunnels](#tcp-tunnels)
- [Client Reference](#client-reference)
- [Server Reference](#server-reference)
- [Updating](#updating)
- [Testing](#testing)
- [Troubleshooting](#troubleshooting)
- [Security Model](#security-model)
- [Limitations](#limitations)
- [Building From Source & Releasing](#building-from-source--releasing)
- [FAQ](#faq)
- [License](#license)

---
## Documentation

| Doc | For whom |
|---|---|
| [Usage Guide](docs/USAGE-GUIDE.md) | Tunnel users & admins — day-to-day usage and acceptable use |
| [Functional Logic](docs/FUNCTIONAL-LOGIC.md) | Developers — how the system works internally |
| [Business Logic](docs/BUSINESS-LOGIC.md) | Operators — accounts, plans, limits, enforcement |
| [Testing Guide](TESTING.md) | Everyone — 20-minute end-to-end test plan |
---
## How It Works

```
                    Internet visitors
                          │
                    HTTPS :443
                          │
                 ┌────────▼────────┐
                 │  Tunnel Server   │   your VPS (Ubuntu)
                 │  tun.example.com │
                 └────────┬────────┘
                          │
                 TLS control channel :7000
                 (client dials OUT — no
                  inbound port needed)
                          │
                 ┌────────▼────────┐
                 │  Tunnel Client   │   your laptop / Pi / office PC
                 └────────┬────────┘
                          │
                   localhost:8000     your local app
```

The client keeps one outbound TLS connection to the server. Public requests
to `https://<subdomain>.tun.example.com` are multiplexed back through that
connection to your local service. TCP works the same way via a dedicated
public port (e.g. `tun.example.com:20000`).

## Features

| Category | Details |
|---|---|
| Protocols | HTTP/HTTPS tunnels, raw TCP tunnels, WebSockets |
| TLS | Automatic Let's Encrypt certificates, TLS control channel |
| Auth | 192-bit tokens (only the SHA-256 hash is stored), token reset |
| Accounts | Enable/disable, expiry dates, plans, per-user bandwidth limits |
| Admin | Web dashboard (localhost-only, bcrypt + rate-limited + CSRF) |
| Ops | systemd hardening, JSON logs, audit log, metrics endpoint, SQLite |
| Client | Auto-reconnect with backoff, config file, env vars, all major OSes |
| Releases | GitHub Actions builds + SHA-256 checksums |

---

## Quick Start (10 minutes)

**You need:** a domain, an Ubuntu 22.04/24.04 VPS with a public IPv4
(1 GB RAM is fine), and root SSH access.

```bash
# ── On the VPS ────────────────────────────────────────────────
curl -fsSL -o install.sh https://raw.githubusercontent.com/migandhi/tunnel/main/deploy/install.sh
sudo bash install.sh
# (answers: your domain, your email, an admin password)
```

```powershell
# ── On your Windows laptop ────────────────────────────────────
irm https://raw.githubusercontent.com/migandhi/tunnel/main/deploy/get-client.ps1 | iex

# open admin dashboard through SSH (leave this window open):
ssh -L 9800:127.0.0.1:9800 root@tun.example.com
# → browse http://127.0.0.1:9800, create a user, copy the token

# start something local, then tunnel it:
python -m http.server 8000
tunnel-client http 8000 --server tun.example.com:7000 --token tk_YOURTOKEN
```

Open the printed URL (e.g. `https://myapp.tun.example.com`). Done. 🎉

Details for every step are below.

---

## Step 1 — DNS Records

Create **two A records** pointing at your VPS IP. The wildcard is **required**
— every tunnel gets its own subdomain.

| Type | Hostname | Value | TTL |
|---|---|---|---|
| A | `tun.example.com` | `203.0.113.10` (your VPS IP) | 300 |
| A | `*.tun.example.com` | `203.0.113.10` (your VPS IP) | 300 |

**DigitalOcean example** (domain managed in DO → Networking → Domains):
- Hostname `tun`, will direct to your droplet
- Hostname `*.tun`, same droplet IP

**Verify before installing** (from any machine):

```powershell
nslookup tun.example.com
nslookup anything.tun.example.com
```

Both must return your VPS IP. If they don't, wait for DNS propagation —
Let's Encrypt certificate issuance **will fail** until DNS is correct.

> You can use a subdomain of an existing domain (`tun.mysite.com`) — you do
> not need a dedicated domain.

---

## Step 2 — Install the Server

On the VPS, as root:

```bash
curl -fsSL -o install.sh https://raw.githubusercontent.com/migandhi/tunnel/main/deploy/install.sh
sudo bash install.sh
```

The installer:

1. Downloads the latest release binary for your CPU (amd64/arm64)
2. Creates an unprivileged `tunnel` system user
3. Asks for: **domain**, **Let's Encrypt email**, **admin username**, **admin password** (min 12 chars — only the bcrypt hash is stored)
4. Writes `/etc/tunnel/server.env` (permissions `640`)
5. Checks your DNS actually points at this VPS and warns if not
6. Installs a hardened systemd service and starts it
7. Offers to add UFW firewall rules

**Ports used:**

| Port | Purpose | Public? |
|---|---|---|
| 80 | HTTP→HTTPS redirect + ACME challenges | ✅ Yes |
| 443 | Public HTTPS tunnel traffic | ✅ Yes |
| 7000 | Client control channel (TLS) | ✅ Yes |
| 20000–20249 | Public TCP tunnel ports | ✅ Yes |
| 9800 | Admin dashboard | ❌ **localhost only — never expose** |

If UFW isn't managing your firewall, open the public ports in your cloud
firewall (DigitalOcean → Networking → Firewalls).

**Verify:**

```bash
systemctl status tunnel-server --no-pager
journalctl -u tunnel-server -f          # live logs
ss -tlnp | grep -E ':80|:443|:7000|:9800'
curl -I https://tun.example.com          # first request triggers cert issuance; may take ~10s
```

---

## Step 3 — Open the Admin Dashboard

The dashboard is **deliberately localhost-only** on the VPS. Reach it via an
SSH tunnel — Windows 10/11, macOS, and Linux all have `ssh` built in:

```powershell
ssh -L 9800:127.0.0.1:9800 root@tun.example.com
```

Keep that terminal open, then browse to:

```
http://127.0.0.1:9800
```

Log in with your admin username/password. From the dashboard you can:

- **Create users** — pick a subdomain, plan, expiry days, GB limit, optional TCP port
- **See status** — Online / Offline / Expired / Over limit, live bandwidth bars
- Renew, disable/enable, delete, **reset tokens**, reset bandwidth, change limits
- View JSON metrics (`/metrics`) and the audit log (`/audit`)

When you create a user, the **token is shown exactly once** (only its hash is
stored). Copy it immediately. Lost token → click **New token**.

---

## Step 4 — Install the Client

### Windows (PowerShell)

```powershell
irm https://raw.githubusercontent.com/migandhi/tunnel/main/deploy/get-client.ps1 | iex
```

Installs `tunnel-client.exe` to `%USERPROFILE%\tunnel` and adds it to your
PATH (open a **new** terminal afterwards).

### macOS / Linux

```bash
curl -fsSL https://raw.githubusercontent.com/migandhi/tunnel/main/deploy/get-client.sh | bash
```

Installs to `/usr/local/bin/tunnel-client`.

### Manual download

Grab the right binary from
[**Releases**](https://github.com/migandhi/tunnel/releases):

```
tunnel-client-windows-amd64.exe    tunnel-client-windows-arm64.exe
tunnel-client-darwin-amd64         tunnel-client-darwin-arm64   (macOS)
tunnel-client-linux-amd64          tunnel-client-linux-arm64
```

Verify integrity against the `SHA256SUMS` file in the release:

```bash
sha256sum -c SHA256SUMS                                  # Linux/macOS
Get-FileHash .\tunnel-client-windows-amd64.exe           # Windows
```

> macOS Gatekeeper: `chmod +x tunnel-client && xattr -d com.apple.quarantine tunnel-client`

---

## Step 5 — Start a Tunnel

Run any local web app — for a quick test:

```bash
python -m http.server 8000
```

Then:

```bash
tunnel-client http 8000 --server tun.example.com:7000 --token tk_YOURTOKEN
```

Output:

```
Connected
Public URL: https://myapp.tun.example.com
Status: Online
```

Open the URL from **any device, anywhere**. The client auto-reconnects if the
connection drops (2s → 60s backoff). Press `Ctrl+C` to stop.

### Save your settings (recommended)

Create `~/.tunnel/config` (Windows: `C:\Users\YOU\.tunnel\config`):

```ini
server = tun.example.com:7000
token  = tk_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
```

Then it's just:

```bash
tunnel-client http 8000
```

---

## TCP Tunnels

For SSH, game servers, databases, RDP, MQTT — anything TCP.

1. In the admin dashboard, create (or edit) a user with a **TCP port**
   (choose *auto-assign*). E.g. it gets port `20000`.
2. Run the client against your local TCP service:

```bash
tunnel-client tcp 25565 --server tun.example.com:7000 --token tk_YOURTOKEN
```

```
Public endpoint: tun.example.com:20000
```

Anyone connecting to `tun.example.com:20000` reaches your local `:25565`.
Idle TCP connections are closed after 5 minutes (configurable).

> UDP is **not** supported.

---

## Client Reference

```
tunnel-client http <local-port> [flags]
tunnel-client tcp  <local-port> [flags]
tunnel-client version
```

| Flag | Meaning |
|---|---|
| `--server host:port` | Tunnel server control address (e.g. `tun.example.com:7000`) |
| `--token tk_...` | Your auth token |
| `--local-host addr` | Forward to a host other than `127.0.0.1` |
| `--insecure` | Skip TLS certificate verification — **testing only** |
| `--no-tls` | Plaintext control channel — **dev mode only** |

**Settings priority:** flags → environment variables → `~/.tunnel/config`.

Environment variables: `TUNNEL_SERVER`, `TUNNEL_TOKEN`, `TUNNEL_NO_TLS=1`.

---

## Server Reference

### Files on the VPS

```
/usr/local/bin/tunnel-server                  binary
/etc/tunnel/server.env                        config (chmod 640, keep private)
/etc/systemd/system/tunnel-server.service     service unit
/var/lib/tunnel/tunnel.db                     SQLite database
/var/lib/tunnel/certs/                        Let's Encrypt certificates
```

### Configuration (`/etc/tunnel/server.env`)

| Variable | Default | Notes |
|---|---|---|
| `TUNNEL_DOMAIN` | *(required)* | e.g. `tun.example.com` |
| `TUNNEL_ACME_EMAIL` | | Let's Encrypt account email |
| `TUNNEL_ADMIN_USER` | `admin` | |
| `TUNNEL_ADMIN_PASS_HASH` | *(required)* | Generate: `tunnel-server hash-password` |
| `TUNNEL_TLS_MODE` | `auto` | `auto` (Let's Encrypt) / `static` / `off` (dev only) |
| `TUNNEL_DATA_DIR` | `/var/lib/tunnel` | |
| `TUNNEL_TCP_PORT_MIN` / `MAX` | `20000` / `20249` | TCP tunnel port range |
| `TUNNEL_MAX_STREAMS` | `128` | Concurrent streams per tunnel |
| `TUNNEL_STREAM_WINDOW_KB` | `1024` | Lower (e.g. `256`) to save RAM on tiny VPSes |
| `TUNNEL_ADMIN_ADDR` | `127.0.0.1:9800` | Keep on localhost |

After editing: `sudo systemctl restart tunnel-server`

### Useful commands

```bash
journalctl -u tunnel-server -f            # live logs
tunnel-server version                     # installed version
tunnel-server hash-password               # generate admin bcrypt hash
sqlite3 /var/lib/tunnel/tunnel.db .backup /root/tunnel-backup.db   # backup
```

### Local development mode (no VPS, no TLS)

```powershell
$env:TUNNEL_DEV="1"
$env:TUNNEL_ADMIN_USER="admin"
$env:TUNNEL_ADMIN_PASS_HASH="<bcrypt hash>"
.\tunnel-server.exe run     # HTTP proxy on :8080, control :7000, admin :9800
```

Client: add `--no-tls --server localhost:7000`. **Never expose dev mode
to the internet.**

---

## Updating

### Server (one command)

```bash
curl -fsSL -o upgrade.sh https://raw.githubusercontent.com/migandhi/tunnel/main/deploy/upgrade.sh
sudo bash upgrade.sh              # latest release
sudo bash upgrade.sh v1.1.0       # or a specific version
```

The upgrade script **backs up the binary and the database first**, installs
the new binary, restarts the service, and prints rollback instructions if the
service fails to start. Config, users, tokens, and certificates are untouched.

### Client

Just re-run the install one-liner (Step 4) — it always fetches the latest
release. Or download the new binary manually.

### Version compatibility

The client and server negotiate a protocol version at connect. If they
mismatch, the client prints a clear "please upgrade" message instead of
misbehaving.

---

## Testing

See [TESTING.md](TESTING.md) for the full 20-minute end-to-end test plan that
needs only **one Windows laptop and one VPS**.

Smoke test in 60 seconds after any change:

```bash
# on the VPS
systemctl is-active tunnel-server && curl -sI https://tun.example.com | head -1

# on the laptop
tunnel-client http 8000    # then open the printed URL
```

Developers: `go test ./... && go vet ./...` must pass before any release.

---

## Troubleshooting

| Symptom | Fix |
|---|---|
| `authentication failed: unknown token` | Wrong/reset token — get a fresh one from the admin dashboard |
| `subscription expired` / `account is disabled` | Renew/enable the user in the dashboard |
| Browser: **Tunnel offline** | Client isn't running/connected — check the client terminal |
| Browser: **Tunnel error** (502) | Client is connected but your local app on that port isn't responding |
| Certificate errors on first visit | Wait ~15s (first-visit cert issuance) and retry; verify wildcard DNS |
| Client can't connect to `:7000` | Firewall — `sudo ufw status`; also check DO cloud firewall |
| Admin UI unreachable | It's localhost-only by design — use the SSH tunnel (`ssh -L 9800:...`) |
| Install script DNS warning | Fix your A records (Step 1) before continuing |
| Service won't start | `journalctl -u tunnel-server -n 50 --no-pager` — usually a bad env var |

---

## Security Model

**Protections built in:** TLS everywhere (control + public), 192-bit random
tokens stored only as SHA-256 hashes, constant-time comparisons, bcrypt admin
password with per-IP rate limiting and CSRF protection, localhost-only admin
UI, handshake timeouts/size limits/rate limits, strict subdomain validation
with a reserved-name list, parameterized SQL, per-tunnel stream caps, bounded
memory windows, TCP idle timeouts, restricted TCP port range, systemd
sandboxing (`NoNewPrivileges`, `ProtectSystem=strict`, dedicated user,
`CAP_NET_BIND_SERVICE` instead of root, `MemoryMax`), and an audit log.

**Honest caveats:**

- TLS terminates on **your** VPS — the VPS operator can technically see
  tunneled plaintext. Inherent to all reverse-proxy tunnels (ngrok included).
- Anyone with a valid token can expose *their* localhost through *your*
  domain. Only issue tokens to people you trust; you are responsible for
  abuse handling on your domain/IP.
- Never commit: `.env` files, `*.db`, `certs/`, tokens, passwords
  (`.gitignore` already covers these).

Report vulnerabilities privately — see [SECURITY.md](SECURITY.md).

---

## Limitations

Be realistic about what a $6 VPS can do:

- **Single node.** No clustering, no HA, no failover. VPS down = all tunnels down.
- **No UDP.** TCP and HTTP/HTTPS only.
- **Scale:** a 1 GB droplet comfortably handles a modest number of
  light/medium tunnels — it is not a CDN or a high-traffic edge.
- **DDoS:** a small VPS cannot absorb a serious attack. Consider your
  provider's network protections for anything critical.
- **Let's Encrypt rate limits:** each subdomain gets its own certificate on
  first visit. LE allows ~50 new certificates per registered domain per week
  — fine for normal use, a constraint if you mass-create tunnels.
- **TCP tunnels are capped** by the port range (default 250 ports).
- **Bandwidth accounting** flushes to disk every 15s — a hard crash can lose
  the last few seconds of counters (never more).
- **No billing/payments** — the admin manages plans/expiry manually by design.
- **VPS bandwidth is your cost:** every byte through a tunnel counts against
  your provider's transfer quota **twice** (in + out).
- **No client auto-update** (updating is one command, though).

---

## Building From Source & Releasing

Requires Go 1.22+.

```bash
git clone https://github.com/migandhi/tunnel.git
cd tunnel
go test ./... && go vet ./...
go build -o tunnel-server ./cmd/tunnel-server
go build -o tunnel-client ./cmd/tunnel-client
```

Windows: `go build -o tunnel-client.exe ./cmd/tunnel-client`

> The Go module path is `github.com/migandhi/tunnel-software` while the repo
> is `migandhi/tunnel`. This is intentional and harmless — Go builds from the
> local checkout regardless.

### Releasing (maintainers) — zero manual builds

GitHub Actions builds all 8 binaries + `SHA256SUMS` automatically when you
push a version tag:

```bash
git add -A
git commit -m "describe the change"
git push origin main

git tag v1.0.2
git push origin v1.0.2      # ← Actions builds and publishes the release
```

That's the entire release process. Install/upgrade scripts always fetch
"latest", so users get updates with one command and **you never edit the
scripts for a new version**.

Repo layout:

```
cmd/tunnel-server/      server entry point
cmd/tunnel-client/      client entry point
internal/               admin, auth, bandwidth, config, control, proxy,
                        security, store, version
deploy/                 install.sh, upgrade.sh, get-client.sh,
                        get-client.ps1, systemd unit
.github/workflows/      release automation
```

---

## FAQ

**Is this free?** Yes — MIT licensed. Your only cost is the VPS (~$4–6/mo)
and a domain.

**Do WebSockets work?** Yes, through HTTP tunnels.

**Can I run the client on a Raspberry Pi / home server?** Yes —
`tunnel-client-linux-arm64`. Add a systemd unit or cron `@reboot` to keep it
running.

**Can multiple people use one server?** Yes — create one user per person in
the dashboard; each gets their own subdomain, token, limits, and expiry.

**Can I choose the subdomain?** The admin picks it at user creation; the
user's URL is fixed to `https://<subdomain>.<domain>`.

**Does the client machine need any open ports?** No. Outbound-only.

**Where is my data?** Everything lives in `/var/lib/tunnel` on your VPS.
Back that directory up and you can rebuild the server in minutes.

---

## License

MIT — see [LICENSE](LICENSE). © 2026 Murtaza Gandhi.
