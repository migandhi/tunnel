# Testing Guide

Complete end-to-end test using **one Windows laptop + one Ubuntu VPS**.
Total time: ~20 minutes. Placeholders: `tun.example.com`, VPS IP `203.0.113.10`.

## Phase 0 — Prerequisites (5 min)

1. DNS records exist and resolve (run on the laptop):
```powershell
   nslookup tun.example.com          # must return your VPS IP
   nslookup randomname.tun.example.com   # must ALSO return your VPS IP
```
2. You can SSH to the VPS: `ssh root@tun.example.com`

## Phase 1 — Server install (5 min)

On the VPS:

```bash
curl -fsSL -o install.sh https://raw.githubusercontent.com/migandhi/tunnel/main/deploy/install.sh
sudo bash install.sh
```

**PASS criteria:**

```bash
systemctl is-active tunnel-server                 # → active
ss -tlnp | grep -E ':80|:443|:7000|:9800'         # all four listening; 9800 on 127.0.0.1 ONLY
curl -I https://tun.example.com                    # → HTTP/2 200 (first call may take ~15s for the cert)
curl -I http://tun.example.com                     # → 301 redirect to https
```

## Phase 2 — Admin dashboard (3 min)

On the laptop (leave this window open):

```powershell
ssh -L 9800:127.0.0.1:9800 root@tun.example.com
```

Browse `http://127.0.0.1:9800` → log in.

- ✅ Wrong password → 401; correct password → dashboard
- Create user: subdomain `myapp`, plan `free`, 30 days, limit `1` GB, TCP **auto-assign**
- ✅ Token page appears — **copy the token**
- ✅ Trying to browse `http://203.0.113.10:9800` directly from the laptop **fails** (localhost-only — this is a PASS)

## Phase 3 — HTTP tunnel (3 min)

On the laptop, two terminals:

```powershell
# Terminal 1 — any local web app; simplest:
python -m http.server 8000

# Terminal 2:
irm https://raw.githubusercontent.com/migandhi/tunnel/main/deploy/get-client.ps1 | iex
tunnel-client http 8000 --server tun.example.com:7000 --token tk_YOURTOKEN
```

**PASS criteria:**

- Client prints `Connected` and `Public URL: https://myapp.tun.example.com`
- URL opens in a browser with a **valid HTTPS padlock**
- Bonus: open the URL from your **phone on mobile data** → works from anywhere
- Dashboard shows the user as **Online**, bandwidth counter increases after refresh (~15s flush)

## Phase 4 — Resilience (2 min)

1. `Ctrl+C` the client → refresh the public URL → friendly **"Tunnel offline"** page (not a browser error)
2. Restart the client → reconnects, URL works again
3. On the VPS: `sudo systemctl restart tunnel-server` → within ~60s the client auto-reconnects on its own (watch its terminal)

## Phase 5 — Enforcement (3 min)

In the dashboard:

1. **Disable** the user → client terminal shows a fatal "account is disabled" message and exits. **Enable** → client works again on restart.
2. **New token** → old token now fails with "authentication failed"; new token works.
3. **Set limit** to `1` GB, then transfer >1 GB through the tunnel (download a large file via the public URL a few times) → status becomes **Over limit** and the tunnel is disconnected within ~30s. **Reset BW** to restore.

## Phase 6 — TCP tunnel (optional, 3 min)

Your user has an auto-assigned port (e.g. `20000`). Test using SSH-to-nothing
or any TCP service; simplest with Python on the laptop:

```powershell
# Terminal 1 — trivial TCP echo on 25565:
python -c "import socketserver;h=type('H',(socketserver.StreamRequestHandler,),{'handle':lambda s:s.wfile.write(s.rfile.readline())});socketserver.TCPServer(('127.0.0.1',25565),h).serve_forever()"

# Terminal 2:
tunnel-client tcp 25565 --server tun.example.com:7000 --token tk_YOURTOKEN
```

From the VPS (i.e. "the internet"): `echo hello | nc tun.example.com 20000` → prints `hello`. ✅

## Phase 7 — Upgrade path (2 min)

On the VPS:

```bash
curl -fsSL -o upgrade.sh https://raw.githubusercontent.com/migandhi/tunnel/main/deploy/upgrade.sh
sudo bash upgrade.sh
tunnel-server version
```

✅ Service active after upgrade; existing user/token still works; backups
exist (`/usr/local/bin/tunnel-server.bak`, `/var/lib/tunnel/tunnel.db.bak`).

## Developer checks (before tagging any release)

```bash
go mod tidy && go test ./... && go vet ./... && go build ./...
```
