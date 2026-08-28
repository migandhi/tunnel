# Functional Logic — How Tunnel Works Internally

Technical reference for contributors and curious operators. Everything here
maps directly to the source in `internal/` and `cmd/`.

## 1. Component Map

```
cmd/tunnel-server ──► wires everything together, runs 5 concurrent services:
│
├── internal/control    Control server (:7000) — client handshake, auth,
│                       yamux session ownership, tunnel Registry
├── internal/proxy      HTTPProxy (:443/:80) + TCPManager (:20000-20249)
├── internal/admin      Dashboard (127.0.0.1:9800), embedded HTML templates
├── internal/bandwidth  Accountant — in-memory byte counters, periodic flush
├── internal/store      SQLite (users, audit_log), migrations, validation
├── internal/auth       Token generate/hash, admin BasicAuth (bcrypt)
├── internal/security   Sliding-window per-IP rate limiter
└── internal/version    Version string + wire ProtocolVersion (currently 1)

cmd/tunnel-client ──► dial, handshake, accept yamux streams, pipe to localhost
```

## 2. Connection Lifecycle (the core flow)

### 2.1 Client → Server handshake

1. Client dials `server:7000` (TLS unless `--no-tls` dev mode).
2. Client sends **one JSON line**:

```json
{"proto":1,"token":"tk_...","protocol":"http","client_version":"v1.0.1"}
```

3. Server (with a 10s deadline and 8 KB line limit):
   - per-IP rate limit check (default 12 handshakes/min/IP)
   - `proto` must equal server's `ProtocolVersion` → else `unsupported_proto`
   - `protocol` must be `http` or `tcp`
   - `SHA256(token)` looked up in `users.token_hash` → else `invalid_token`
   - business checks in order: `disabled` → `expired` → `bandwidth_exceeded`
     → (tcp only) `no_tcp_port`
4. Server replies with one JSON line:

```json
{"ok":true,"subdomain":"myapp","url":"https://myapp.tun.example.com",
 "warning":"subscription expires in 3 day(s)..."}
```

Error responses carry `error_code`; the client treats these codes as **fatal**
(exit, don't retry): `invalid_token, disabled, expired, bandwidth_exceeded,
unsupported_proto, bad_request, no_tcp_port`. Anything else (network blip)
triggers exponential-backoff reconnect (2s→60s, reset after 60s uptime).

### 2.2 Multiplexing (yamux)

After the handshake, the single TCP/TLS connection is upgraded to a
**yamux session**:

- Server = yamux *server*, client = yamux *client* — but the **server opens
  streams** (one per public request/connection) and the **client accepts** them.
- Keep-alive ping every 20s → dead connections detected within ~30s.
- `MaxStreamWindowSize` = `TUNNEL_STREAM_WINDOW_KB` (bounds per-stream RAM).
- Per-tunnel concurrent stream cap (`TUNNEL_MAX_STREAMS`, default 128),
  enforced by an atomic counter in `control.Tunnel.OpenStream()`.

### 2.3 Registry

`control.Registry` is the in-memory routing table:

```
http map: subdomain → *Tunnel
tcp  map: port      → *Tunnel
```

- Registering over an existing entry **closes the old session** (last
  connection wins — this is how "one tunnel per account" works).
- Lookups return `nil` if the session is closed → callers render
  "Tunnel offline".
- `CloseUser(id)` closes every session of a user (used by admin disable/
  delete/token-reset and by the enforcer).

## 3. HTTP Request Path

```
Browser ── HTTPS :443 ──► HTTPProxy.ServeHTTP
  1. host = lowercase(Host header minus port)
  2. host == base domain?           → serve status page
  3. SubdomainOf(host, domain)?     → exactly one label, else 404
  4. Registry.GetHTTP(sub) == nil?  → 503 "Tunnel offline" page
  5. httputil.ReverseProxy with a custom Transport whose DialContext
     opens a NEW yamux stream to that tunnel, wrapped by the bandwidth
     Accountant (counts every byte read+written)
  6. Rewrite: X-Forwarded-* set, original Host preserved to the local app
```

- `FlushInterval: -1` → streaming/SSE flushed immediately.
- WebSockets: Go's ReverseProxy handles the `Upgrade` hijack natively; the
  raw duplex bytes ride the same yamux stream.
- Errors from the local app → branded 502 page; client-canceled requests are
  ignored.

Port 80 behavior (TLS on): ACME HTTP-01 challenges are answered, everything
else gets a 301 to HTTPS.

## 4. TCP Path

```
Public conn :20000 ──► TCPManager.bridge
  1. Registry.GetTCP(20000) — nil? drop connection
  2. Open yamux stream (respects stream cap), wrap with Accountant
  3. Bidirectional io.Copy; each side wrapped with a rolling idle
     deadline (TUNNEL_TCP_IDLE_TIMEOUT, default 5m)
  4. Either side closing tears down both
```

Listeners are bound **lazily** on first tunnel connect (`EnsurePort`), kept
open across client reconnects, and released on user delete (`ClosePort`).
Range enforced: `TCP_PORT_MIN..MAX`.

## 5. TLS / Certificates

`TUNNEL_TLS_MODE=auto` (production):

- `autocert.Manager` with `DirCache(/var/lib/tunnel/certs)`.
- **HostPolicy** only permits: the base domain, or a single-label subdomain
  that **exists in the users table** — prevents attackers from burning
  Let's Encrypt rate limits with garbage hostnames.
- Same certificate source serves :443 (h2+http/1.1+ALPN-01) and :7000.
- Certificates are issued **on first request** per subdomain (~5-15s once).

`static` mode: operator-provided cert/key (must cover the wildcard).
`off`: dev only; refused unless `TUNNEL_DEV=1`.

## 6. Bandwidth Accounting

```
trackedConn.Read/Write ──► Accountant.Add(userID, n)   (mutex + map, in-memory)
                                    │
                        every 15s / shutdown / dashboard load
                                    ▼
                    UPDATE users SET bandwidth_used += ?   (per user)
```

- Failed flushes re-queue the bytes (no silent loss on transient DB errors).
- Hard crash loses at most one flush interval (~15s) of counters.
- Both directions count; a byte through the tunnel is counted once at the
  server↔client stream boundary.

## 7. The Enforcer Loop

Every `TUNNEL_ENFORCE_INTERVAL` (30s):

1. `Accountant.FlushNow()` — so decisions use fresh numbers
2. For each user: disabled? expired? over limit?
3. If yes and they have live sessions → `Registry.CloseUser` + audit entry

So limits are enforced **mid-session**, worst case ~30-45s after the
condition becomes true. The handshake enforces the same rules at connect
time, so violators can't reconnect.

## 8. Persistence (SQLite)

- Driver: `modernc.org/sqlite` (pure Go — no CGO, easy cross-compile).
- WAL mode, `busy_timeout=5000`, single connection (`SetMaxOpenConns(1)`)
  — correct and plenty for this write volume.
- Versioned migrations in `schema_version`; append new statement groups to
  the `migrations` slice to evolve the schema safely.

Tables:

```sql
users(id, email, subdomain UNIQUE, token_hash UNIQUE, plan, active,
      tcp_port, bandwidth_used, bandwidth_limit,
      starts_at, expires_at, created_at, updated_at)
audit_log(id, ts, actor, action, detail)
```

All queries parameterized. Times stored as RFC3339 UTC strings.

## 9. Security Mechanics (where in code)

| Control | Location |
|---|---|
| Token = 24 random bytes, hex, `tk_` prefix; stored as SHA-256 | `internal/auth/token.go` |
| Constant-time comparisons | `subtle.ConstantTimeCompare` |
| Handshake: 10s deadline, 8 KB max line, per-IP limiter | `internal/control/server.go` |
| Admin: bcrypt(cost 12) + 20 attempts/min/IP + per-boot CSRF token | `internal/auth/admin.go`, `internal/admin` |
| Subdomain regex + reserved-name list (`www`, `admin`, `api`, …) | `internal/store/validate.go` |
| ACME host allow-list | `cmd/tunnel-server/main.go` HostPolicy |
| systemd sandbox + `CAP_NET_BIND_SERVICE` + `MemoryMax=768M` | `deploy/systemd/…` |

## 10. Shutdown Behavior

`SIGINT/SIGTERM` → cancel context → TCP listeners closed → final bandwidth
flush → exit. yamux sessions drop; clients auto-reconnect when the server
returns (systemd `Restart=always`, `RestartSec=3`).
