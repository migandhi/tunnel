# Tunnel — Usage Guide

Everything you need to **use** the tunneling service day to day.

- **Part A** — For tunnel users (people running the client)
- **Part B** — For the administrator (person running the server)
- **Part C** — Usage guidelines & acceptable use

---

# Part A — For Tunnel Users

## What you receive from the administrator

| Item | Example | Purpose |
|---|---|---|
| Server address | `tun.example.com:7000` | Where the client connects |
| Auth token | `tk_a1b2c3...` (51 chars) | Your identity — treat like a password |
| Public URL | `https://myapp.tun.example.com` | Where the world reaches your app |
| TCP port (optional) | `tun.example.com:20000` | Only if your plan includes TCP |

> ⚠️ **Your token is shown only once when created.** If you lose it, ask the
> admin for a reset — the old token stops working immediately.

## One-time setup

**Windows (PowerShell):**
```powershell
irm https://raw.githubusercontent.com/migandhi/tunnel/main/deploy/get-client.ps1 | iex
```

**macOS / Linux:**
```bash
curl -fsSL https://raw.githubusercontent.com/migandhi/tunnel/main/deploy/get-client.sh | bash
```

**Save your credentials** so you never type them again.
Create the file `~/.tunnel/config`
(Windows: `C:\Users\YOU\.tunnel\config`, no file extension):

```ini
server = tun.example.com:7000
token  = tk_your_token_here
```

## Everyday usage

### Expose a local website / API / dev server

```bash
tunnel-client http 8000
```

Whatever runs on `localhost:8000` (React dev server, Flask, Node, WordPress,
IIS…) is now live at your public HTTPS URL. WebSockets work automatically.

```
Connected
Public URL: https://myapp.tun.example.com
Status: Online
```

Press `Ctrl+C` to stop. The URL goes offline immediately; visitors see a
friendly "Tunnel offline" page.

### Expose a TCP service (SSH, game server, database, RDP…)

Requires a plan with a TCP port.

```bash
tunnel-client tcp 25565
```

```
Public endpoint: tun.example.com:20000
```

Anyone connecting to `tun.example.com:20000` reaches your local `:25565`.

### Forward to another machine on your LAN

```bash
tunnel-client http 80 --local-host 192.168.1.50
```

## Things to know

- **Auto-reconnect:** if Wi-Fi drops or the server restarts, the client
  retries automatically (2s → 4s → … → 60s). Just leave it running.
- **One tunnel per account per protocol:** starting a second HTTP client with
  the same token disconnects the first one (last connection wins).
- **Warnings:** the client tells you at connect time if your subscription
  expires within 7 days or you've used ≥80% of your bandwidth.
- **Clear errors:** "expired", "disabled", "bandwidth limit reached",
  "invalid token" — all mean *contact your administrator*. The client exits
  instead of retrying pointlessly.

## Keep a tunnel running permanently

**Linux (systemd) — e.g. Raspberry Pi / home server:**

```ini
# /etc/systemd/system/my-tunnel.service
[Unit]
Description=My tunnel
After=network-online.target

[Service]
ExecStart=/usr/local/bin/tunnel-client http 8000
Restart=always
User=youruser

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl enable --now my-tunnel
```

(Reads `server`/`token` from that user's `~/.tunnel/config`.)

**Windows:** Task Scheduler → Create Task → Trigger *At log on* →
Action: `C:\Users\YOU\tunnel\tunnel-client.exe` with arguments `http 8000`.

## Quick troubleshooting

| You see | Meaning | Do this |
|---|---|---|
| `authentication failed: unknown token` | Wrong or reset token | Ask admin for a new token |
| `subscription expired on …` | Account expired | Ask admin to renew |
| `account is disabled` | Admin disabled you | Contact admin |
| `bandwidth limit reached` | Used your quota | Ask admin to reset/increase |
| `cannot reach local service 127.0.0.1:8000` | Your local app isn't running | Start your app on that port |
| Visitors see "Tunnel error" (502) | Tunnel is up, local app crashed/hung | Restart your local app |
| Visitors see "Tunnel offline" | Client isn't running | Start `tunnel-client` |

---

# Part B — For the Administrator

## Daily driver: the dashboard

```bash
ssh -L 9800:127.0.0.1:9800 root@tun.example.com    # keep open
# browse http://127.0.0.1:9800
```

## Common admin workflows

**Onboard a new user**
1. Dashboard → *Create user*: subdomain, plan, days, GB limit, TCP (none/auto)
2. Copy the one-time token page contents
3. Send the token to the user over a **secure channel** (not email plaintext)
4. Send them Part A of this guide

**User lost their token** → row → **New token** → send new one. Old token dead instantly; their live session is disconnected.

**Monthly renewal** → row → set days → tick *reset BW* → **Renew**.

**Suspend / restore** → **Disable** (kills live tunnels within seconds) / **Enable**.

**User hit bandwidth limit early** → **Reset BW**, or **Set limit** to a higher GB value.

**Offboard** → **Delete** (frees the subdomain and TCP port immediately).

**Health check**
- `/metrics` — JSON: uptime, goroutines, memory, live tunnels & stream counts
- `/audit` — last 200 events: connects, disconnects, auth failures, admin actions
- VPS: `journalctl -u tunnel-server -f`

---

# Part C — Usage Guidelines & Acceptable Use

The server operator's domain and IP reputation are on the line for **all**
traffic. Recommended rules for anyone you give a token to:

**Allowed / intended uses**
- Demos, webhooks testing, client previews of work-in-progress sites
- Home lab / IoT dashboards, personal self-hosted apps
- Game servers with friends, SSH access to your own machines
- Temporary public endpoints during development

**Not allowed**
- Illegal content, phishing, malware distribution, spam infrastructure
- Bypassing the network policies of an employer/institution without permission
- Sharing your token — it identifies **you**; you are responsible for its use
- Sustained high-bandwidth abuse (this runs on a small shared VPS)
- Exposing services with default/no passwords to the public internet

**Security hygiene for users**
- Anything you tunnel is **public**. Add authentication to your local app.
- Don't tunnel production databases or admin panels unless you must — and
  then only TCP + strong auth.
- Rotate your token if a laptop is lost or a `.tunnel/config` file leaks.

**Operator's rights**: the administrator may disable any account, revoke any
token, and inspect audit logs at any time. Bandwidth limits and expiry dates
are enforced automatically.
