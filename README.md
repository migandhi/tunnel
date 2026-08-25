# tunnel

A lightweight self-hosted HTTP/HTTPS and TCP tunnel service for a small VPS.

It is designed for authorized development, demos, homelabs, IoT, game servers,
and other services that need an outbound-only client connection through NAT or
CGNAT.

> **Status:** production-oriented codebase, but you must run the automated tests,
> compile it, and perform the integration checklist before putting real users on it.

## Architecture

```text
Public HTTPS :443
      |
      v
 tunnel-server
      |
      | TLS control connection :7000
      v
 tunnel-client
      |
      v
127.0.0.1:<local-port>
```

HTTP tunnels use a per-user subdomain:

```text
https://myapp.tun.example.com
```

TCP tunnels use an assigned port:

```text
tun.example.com:20001
```

The client makes an outbound connection, so the local machine does not need an
inbound firewall port.

## Repository

This package is preconfigured for the repository:

`https://github.com/migandhi/tunnel-software`

If you choose a different GitHub repository, change the module path in `go.mod`,
the internal import paths, and the repository name in `deploy/install.sh` and
`deploy/upgrade.sh`.

## Requirements

### VPS

- Ubuntu 20.04+ recommended
- 1 GB RAM is suitable for a small deployment
- Public IPv4
- Ports 80, 443, 7000 and 20000-20249 TCP
- A domain you control

### DNS

For `tun.example.com` pointing to VPS `203.0.113.10`:

```text
A    tun.example.com       203.0.113.10
A    *.tun.example.com     203.0.113.10
```

Set:

```text
TUNNEL_DOMAIN=tun.example.com
```

Do not expose the admin port 9800 publicly.

## Build on Windows

Install Go 1.22+ and Git.

Open PowerShell in the repository:

```powershell
go mod tidy
go test ./...
go vet ./...
go build ./...
```

Build local Windows binaries:

```powershell
go build -o tunnel-server.exe ./cmd/tunnel-server
go build -o tunnel-client.exe ./cmd/tunnel-client
```

The server binary is normally deployed to Linux; the client binary is what
Windows users run.

## Push to GitHub

```powershell
git init
git add .
git commit -m "Initial tunnel release"
git branch -M main
git remote add origin https://github.com/migandhi/tunnel-software.git
git push -u origin main
```

Do not commit:

- `.env`
- `*.db`
- certificates
- tokens
- private keys
- production configuration

## Build release binaries

On a Linux/macOS machine with Make:

```bash
make test
make vet
make release
```

The `dist/` directory contains:

```text
tunnel-server-linux-amd64
tunnel-server-linux-arm64
tunnel-client-linux-amd64
tunnel-client-linux-arm64
tunnel-client-darwin-amd64
tunnel-client-darwin-arm64
tunnel-client-windows-amd64.exe
tunnel-client-windows-arm64.exe
SHA256SUMS
```

Create a GitHub Release such as `v1.0.0` and upload the required binaries.

The installer expects the exact filenames above.

## Server installation

On the Ubuntu VPS:

```bash
curl -fsSLO https://raw.githubusercontent.com/migandhi/tunnel-software/main/deploy/install.sh
less install.sh
sudo bash install.sh
```

The installer asks for:

- base tunnel domain
- ACME email
- admin username
- admin password

It creates:

```text
/usr/local/bin/tunnel-server
/etc/tunnel/server.env
/etc/systemd/system/tunnel-server.service
/var/lib/tunnel/tunnel.db
/var/lib/tunnel/certs/
```

Check the service:

```bash
systemctl status tunnel-server
journalctl -u tunnel-server -f
```

## Firewall

Open:

```text
22/tcp
80/tcp
443/tcp
7000/tcp
20000-20249/tcp
```

Do not open 9800.

If using DigitalOcean Cloud Firewall, configure the same ports there.

## Admin UI

The admin UI listens on:

```text
127.0.0.1:9800
```

Create an SSH tunnel from your Windows PC:

```powershell
ssh -L 9800:127.0.0.1:9800 root@YOUR_VPS_IP
```

Then browse:

```text
http://127.0.0.1:9800
```

Create a user. The authentication token is shown once; only its SHA-256 hash
is stored in SQLite.

## First HTTP tunnel

Run a local web service on port 8000.

Then:

```powershell
.	unnel-client.exe http 8000 --server tun.example.com:7000 --token tk_xxxxx
```

The client prints the public URL:

```text
https://myapp.tun.example.com
```

## TCP tunnel

Assign the user a TCP port in the admin UI.

For example, if the assigned port is 20001:

```powershell
.	unnel-client.exe tcp 25565 --server tun.example.com:7000 --token tk_xxxxx
```

The public endpoint is:

```text
tun.example.com:20001
```

## Client configuration file

Create:

```text
~/.tunnel/config
```

with:

```text
server = tun.example.com:7000
token = tk_xxxxx
```

Then:

```bash
tunnel-client http 8000
```

or:

```bash
tunnel-client tcp 25565
```

Command-line flags take precedence over environment variables and config-file
values.

## Client environment variables

```text
TUNNEL_SERVER
TUNNEL_TOKEN
TUNNEL_NO_TLS=1
```

Do not use `TUNNEL_NO_TLS` in production.

## Server configuration

Important variables:

```text
TUNNEL_DOMAIN
TUNNEL_ACME_EMAIL
TUNNEL_ADMIN_USER
TUNNEL_ADMIN_PASS_HASH
TUNNEL_DATA_DIR
TUNNEL_TLS_MODE
TUNNEL_TCP_PORT_MIN
TUNNEL_TCP_PORT_MAX
TUNNEL_MAX_STREAMS
TUNNEL_STREAM_WINDOW_KB
```

TLS modes:

- `auto` — Let's Encrypt
- `static` — your certificate
- `off` — development only

## Security model

Implemented:

- 192-bit random tokens
- SHA-256 token storage
- TLS control channel
- TLS public HTTPS
- handshake timeout
- handshake size limit
- control connection rate limiting
- bcrypt admin password
- admin rate limiting
- CSRF protection
- strict subdomain validation
- parameterized SQLite queries
- bounded yamux stream windows
- per-tunnel concurrent stream limits
- TCP idle timeout
- restricted TCP port range
- localhost-only admin interface
- systemd hardening
- audit log

The VPS operator can see tunneled plaintext after TLS termination. This is
inherent to a reverse-proxy tunnel architecture.

## Bandwidth

Traffic through HTTP and TCP tunnel streams is counted.

Counters are accumulated in memory and periodically flushed to SQLite. The
enforcer disconnects disabled, expired, or over-limit accounts.

Because persistence is periodic, a sudden process failure can lose the most
recent unflushed accounting interval.

## Subscriptions

There is intentionally no payment gateway.

The admin controls:

- plan label
- start/expiry
- bandwidth limit
- enable/disable
- token reset
- bandwidth reset
- TCP port

A future billing system can call the same store operations.

## Upgrades

Use an explicit version for production:

```bash
sudo bash deploy/upgrade.sh v1.1.0
```

The script backs up the binary and SQLite database before upgrading.

## Testing

Before deployment:

```bash
go mod tidy
go test ./...
go vet ./...
go build ./...
```

Then manually test:

1. HTTP tunnel
2. HTTPS certificate issuance
3. WebSockets
4. large file transfer
5. client reconnect
6. server restart
7. account expiry
8. bandwidth enforcement
9. token reset
10. account disable
11. TCP forwarding
12. TCP idle timeout

## Development mode

Development mode disables TLS and moves the HTTP proxy to port 8080:

```bash
TUNNEL_DEV=1 TUNNEL_ADMIN_PASS_HASH='<bcrypt hash>' go run ./cmd/tunnel-server run
```

Client:

```bash
go run ./cmd/tunnel-client http 8000 --server localhost:7000 --token tk_xxxxx --no-tls
```

Use this only for local testing.

## Limitations

This is a single-node system.

A 1 GB VPS is intended for a modest number of light tunnels, not as a high-scale
internet edge. CPU, memory, network transfer and abuse traffic are the practical
limits.

UDP is not supported.

A very large L7 DDoS can overwhelm a small VPS.

The service operator is responsible for abuse handling and acceptable-use
policies.

## Old deployment warning

If you are replacing the original tunnel project, treat all old credentials
as compromised. Rotate the old admin password, old tunnel tokens and any old
Razorpay credentials. Do not copy the old database into this implementation.

## License

MIT.
