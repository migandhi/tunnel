# Tunnel

A lightweight self-hosted HTTP/HTTPS and TCP tunnel service for a small VPS.

Tunnel allows a machine behind NAT, CGNAT, or a restrictive firewall to expose
authorized local services through a public VPS using an outbound connection.

Designed for development, demos, homelabs, IoT, game servers, internal tools,
temporary public endpoints, and small self-hosted services.

> **Status:** Production-oriented and successfully tested on Ubuntu 24.04 with
> HTTP/HTTPS and TCP tunnels. Review the security and operational requirements
> before exposing services to untrusted users.

## Features

- HTTP/HTTPS tunneling
- TCP tunneling
- Outbound-only client connection
- Automatic HTTPS with Let's Encrypt
- Per-user authentication tokens
- SHA-256 token storage
- Admin web interface
- Account enable/disable and expiry
- Bandwidth limits and accounting
- TCP port allocation
- Token and bandwidth reset
- SQLite storage
- TLS control connection
- Automatic client reconnect
- WebSocket support through HTTP proxying
- systemd service support
- Linux, Windows and macOS clients
- Linux server binaries
- SHA-256 release checksums

## Architecture

```text
                         Internet
                            |
                    +-------v--------+
                    |  Tunnel Server |
                    |      VPS      |
                    +-------+-------+
                            |
             +--------------+--------------+
             |                             |
        HTTPS :443                    Control :7000
             |                             |
             |                     TLS connection
             |                             |
             |                      +------v------+
             |                      |Tunnel Client|
             |                      +------+------+
             |                             |
             |                       127.0.0.1:8000
             |                             |
             +-----------------------------+
```

### HTTP tunnel

```text
https://myapp.tun.example.com
              |
              v
       Tunnel Server
              |
              v
       Tunnel Client
              |
              v
     127.0.0.1:8000
```

### TCP tunnel

```text
tun.example.com:20000
          |
          v
    Tunnel Server
          |
          v
    Tunnel Client
          |
          v
  127.0.0.1:25565
```

The client makes an outbound connection to the server. The local machine does
not need to expose an inbound firewall port.

## Repository

GitHub: https://github.com/migandhi/tunnel

Current release: **v1.0.1**

Go module:

```text
github.com/migandhi/tunnel-software
```

> The GitHub repository name and Go module path are currently different.
> The module path is used by the Go source code and release build process.

## Requirements

### Server

- Ubuntu 24.04 LTS recommended
- 1 GB RAM or more
- Public IPv4 address
- Domain name
- Root SSH access for installation
- Ports 80, 443 and 7000
- TCP ports 20000-20249 for TCP tunnels

### Client

Supported platforms:

- Windows amd64
- Windows arm64
- Linux amd64
- Linux arm64
- macOS amd64
- macOS arm64

## DNS Setup

Assume the tunnel server is `tun.example.com` and the VPS IP is
`203.0.113.10`.

Create:

```text
A    tun.example.com       203.0.113.10
A    *.tun.example.com     203.0.113.10
```

The wildcard record is required for HTTP tunnel subdomains.

For example:

```text
https://myapp.tun.example.com
```

must resolve to the VPS.

Set:

```ini
TUNNEL_DOMAIN=tun.example.com
```

## Download a Release

Download the appropriate client from:

https://github.com/migandhi/tunnel/releases

Linux server binaries:

```text
tunnel-server-linux-amd64
tunnel-server-linux-arm64
```

Client binaries:

```text
tunnel-client-linux-amd64
tunnel-client-linux-arm64
tunnel-client-windows-amd64.exe
tunnel-client-windows-arm64.exe
tunnel-client-darwin-amd64
tunnel-client-darwin-arm64
```

## Verify Release Checksums

Each release contains:

```text
SHA256SUMS
```

On Linux:

```bash
sha256sum -c SHA256SUMS
```

On Windows PowerShell:

```powershell
Get-FileHash .\tunnel-client-windows-amd64.exe -Algorithm SHA256
```

Compare the resulting hash with the corresponding value in `SHA256SUMS`.

## Server Installation

### 1. Prepare Ubuntu

```bash
apt update
apt upgrade -y
apt install -y ufw ca-certificates
```

### 2. Create directories

```bash
mkdir -p /etc/tunnel
mkdir -p /var/lib/tunnel/certs
```

### 3. Install the server binary

Copy the release binary to:

```text
/usr/local/bin/tunnel-server
```

Then:

```bash
chmod 755 /usr/local/bin/tunnel-server
/usr/local/bin/tunnel-server version
```

Example:

```text
v1.0.1
```

## Server Configuration

Create:

```text
/etc/tunnel/server.env
```

Example:

```ini
TUNNEL_DOMAIN=tun.example.com
TUNNEL_ACME_EMAIL=admin@example.com
TUNNEL_DATA_DIR=/var/lib/tunnel
TUNNEL_TLS_MODE=auto
TUNNEL_TCP_PORT_MIN=20000
TUNNEL_TCP_PORT_MAX=20249
TUNNEL_MAX_STREAMS=128
TUNNEL_STREAM_WINDOW_KB=256
TUNNEL_ADMIN_USER=admin
TUNNEL_ADMIN_PASS_HASH=$2a$12$YOUR_BCRYPT_HASH
```

Generate a bcrypt hash:

```bash
tunnel-server hash-password
```

Do not store the plaintext admin password in the server configuration.

Protect the configuration:

```bash
chmod 600 /etc/tunnel/server.env
```

## Firewall

Allow:

```text
22/tcp
80/tcp
443/tcp
7000/tcp
20000-20249/tcp
```

Example:

```bash
ufw allow 22/tcp
ufw allow 80/tcp
ufw allow 443/tcp
ufw allow 7000/tcp
ufw allow 20000:20249/tcp
ufw --force enable
ufw status
```

Do **not** expose port 9800 publicly.

## systemd Service

Create:

```text
/etc/systemd/system/tunnel-server.service
```

```ini
[Unit]
Description=Tunnel Server
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=tunnel
EnvironmentFile=/etc/tunnel/server.env
ExecStart=/usr/local/bin/tunnel-server run
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

Create the unprivileged service account:

```bash
useradd --system --no-create-home --shell /usr/sbin/nologin tunnel
```

Give it ownership of application data:

```bash
chown -R tunnel:tunnel /var/lib/tunnel
```

Allow the binary to bind ports 80 and 443 without running the process as root:

```bash
setcap 'cap_net_bind_service=+ep' /usr/local/bin/tunnel-server
```

Verify:

```bash
getcap /usr/local/bin/tunnel-server
```

Expected:

```text
/usr/local/bin/tunnel-server cap_net_bind_service=ep
```

Enable and start:

```bash
systemctl daemon-reload
systemctl enable tunnel-server
systemctl start tunnel-server
systemctl status tunnel-server --no-pager
```

## Server Ports

| Port | Purpose | Public |
|---|---|---|
| 80 | HTTP redirect / ACME | Yes |
| 443 | HTTPS tunnel traffic | Yes |
| 7000 | Client control connection | Yes |
| 9800 | Admin UI | No |
| 20000-20249 | TCP tunnels | Yes |

The admin UI listens only on:

```text
127.0.0.1:9800
```

## Admin UI

The admin interface is intentionally not exposed publicly.

Create an SSH tunnel from your Windows PC:

```powershell
ssh -L 9800:127.0.0.1:9800 root@YOUR_VPS_IP
```

Leave the SSH session running.

Open:

```text
http://127.0.0.1:9800
```

Log in using the configured admin username and password.

The admin interface can manage users, plans, expiry, bandwidth limits,
enable/disable state, tokens, token reset, bandwidth reset, and TCP ports.

## Creating a Tunnel User

Create a user through the admin interface.

The server displays the authentication token once:

```text
tk_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
```

Store the token securely.

Only the token hash is stored by the server. If a token is lost, reset it
from the admin interface.

## HTTP Tunnel

Start a local HTTP service:

```powershell
python -m http.server 8000
```

Verify:

```powershell
curl.exe http://localhost:8000
```

Start the client:

```powershell
.\tunnel-client-windows-amd64.exe http 8000 `
  --server tun.example.com:7000 `
  --token 'YOUR_TOKEN'
```

The client should display a public URL such as:

```text
https://myapp.tun.example.com
```

Open that URL in a browser.

Traffic flows:

```text
Browser
   |
HTTPS :443
   |
Tunnel Server
   |
TLS control connection
   |
Tunnel Client
   |
127.0.0.1:8000
```

## TCP Tunnel

A TCP user must have an assigned TCP port, for example `20000`.

Start a local TCP service on port `25565`, then:

```powershell
.\tunnel-client-windows-amd64.exe tcp 25565 `
  --server tun.example.com:7000 `
  --token 'YOUR_TOKEN'
```

The client displays:

```text
Public endpoint: tun.example.com:20000
```

Connect to:

```text
tun.example.com:20000
```

Traffic is forwarded to:

```text
127.0.0.1:25565
```

## Client Configuration File

The client can use:

```text
~/.tunnel/config
```

Example:

```ini
server = tun.example.com:7000
token = tk_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
```

Then:

```bash
tunnel-client http 8000
```

or:

```bash
tunnel-client tcp 25565
```

Command-line options take precedence over configuration-file values.

## Client Environment Variables

Supported variables:

```text
TUNNEL_SERVER
TUNNEL_TOKEN
TUNNEL_NO_TLS=1
```

Do not use `TUNNEL_NO_TLS=1` in production. It is intended only for
development and local testing.

## TLS

Production control connections use TLS:

```text
Client → TLS → :7000
```

Public HTTP tunnels use HTTPS:

```text
Internet → HTTPS :443
```

When:

```ini
TUNNEL_TLS_MODE=auto
```

the server obtains and manages certificates using ACME/Let's Encrypt.

The domain must resolve correctly before certificate issuance.

## Development Mode

For local development, the server can run without TLS:

```powershell
$env:TUNNEL_DEV="1"
$env:TUNNEL_ADMIN_USER="admin"
$env:TUNNEL_ADMIN_PASS_HASH="YOUR_BCRYPT_HASH"

.\tunnel-server-local.exe run
```

The development server uses:

```text
HTTP proxy: 8080
Admin UI:   127.0.0.1:9800
Control:    7000
```

The client can connect using:

```powershell
.\tunnel-client-local.exe http 8000 `
  --server localhost:7000 `
  --token 'YOUR_TOKEN' `
  --no-tls
```

Development mode should **never be exposed to the public internet**.

## Building from Source

Requirements:

- Go 1.22+
- Git

Clone:

```bash
git clone https://github.com/migandhi/tunnel.git
cd tunnel
```

Run:

```bash
go mod tidy
go test ./...
go vet ./...
go build ./...
```

## Windows Local Build

Set the target:

```powershell
$env:GOOS="windows"
$env:GOARCH="amd64"
$env:CGO_ENABLED="0"
```

Build the server:

```powershell
go build -o tunnel-server-local.exe ./cmd/tunnel-server
```

Build the client:

```powershell
go build -o tunnel-client-local.exe ./cmd/tunnel-client
```

Check:

```powershell
.\tunnel-server-local.exe --help
.\tunnel-client-local.exe --help
```

## Cross-Platform Release Builds

The release workflow builds:

```text
tunnel-server-linux-amd64
tunnel-server-linux-arm64

tunnel-client-linux-amd64
tunnel-client-linux-arm64

tunnel-client-darwin-amd64
tunnel-client-darwin-arm64

tunnel-client-windows-amd64.exe
tunnel-client-windows-arm64.exe
```

Release binaries are generated by GitHub Actions when a tag matching `v*` is
pushed.

Example:

```bash
git tag v1.0.2
git push origin v1.0.2
```

## Testing

Before releasing:

```bash
go mod tidy
go test ./...
go vet ./...
go build ./...
```

Manual integration testing should include:

1. HTTP tunnel
2. HTTPS certificate
3. WebSockets
4. Large file transfer
5. Client reconnect
6. Server restart
7. Account expiry
8. Bandwidth enforcement
9. Token reset
10. Account disable
11. TCP forwarding
12. TCP idle timeout

## Production Verification

Check the service:

```bash
systemctl is-enabled tunnel-server
systemctl is-active tunnel-server
```

Check listening ports:

```bash
ss -tulpn | grep -E ':80|:443|:7000|:9800'
```

Expected:

```text
*:80
*:443
*:7000
127.0.0.1:9800
```

Verify HTTPS:

```bash
curl -I https://tun.example.com
```

Verify HTTP redirect:

```bash
curl -I http://tun.example.com
```

The HTTP endpoint should redirect to HTTPS.

## Security Model

Implemented protections include:

- 192-bit random authentication tokens
- SHA-256 token storage
- TLS control channel
- TLS public HTTPS
- Handshake timeout
- Handshake size limit
- Control connection rate limiting
- bcrypt admin password
- Admin rate limiting
- CSRF protection
- Strict subdomain validation
- Parameterized SQLite queries
- Bounded yamux stream windows
- Per-tunnel concurrent stream limits
- TCP idle timeout
- Restricted TCP port range
- Localhost-only admin interface
- systemd service isolation through a dedicated user
- `CAP_NET_BIND_SERVICE` instead of running the server as root
- Audit logging

The VPS operator can see tunneled plaintext after TLS termination. This is
inherent to a reverse-proxy tunnel architecture.

## Bandwidth

Traffic through HTTP and TCP tunnel streams is counted.

Counters are accumulated in memory and periodically flushed to SQLite.

The enforcer disconnects accounts that are disabled, expired, or over their
bandwidth limit.

Because persistence is periodic, a sudden process failure can lose the most
recent unflushed accounting interval.

## Subscriptions

There is intentionally no payment gateway in the tunnel server.

The admin controls:

- Plan label
- Start date
- Expiry date
- Bandwidth limit
- Enable/disable state
- Token reset
- Bandwidth reset
- TCP port

A future billing system can integrate with the same store operations.

## Upgrades

Use an explicit release version:

```bash
sudo bash deploy/upgrade.sh v1.1.0
```

The upgrade process should back up the binary and SQLite database before
replacing the running version.

Always verify after an upgrade:

```bash
systemctl status tunnel-server --no-pager
journalctl -u tunnel-server -n 50 --no-pager
```

## Data and Configuration

Production files:

```text
/usr/local/bin/tunnel-server
/etc/tunnel/server.env
/etc/systemd/system/tunnel-server.service
/var/lib/tunnel/tunnel.db
/var/lib/tunnel/certs/
```

Do not commit:

```text
.env
*.db
*.db-wal
*.db-shm
certs/
private keys
authentication tokens
production passwords
production configuration
```

## Limitations

This is a single-node system.

A 1 GB VPS is intended for a modest number of light tunnels, not a high-scale
internet edge.

Practical limits include CPU, RAM, network transfer, concurrent streams, VPS
bandwidth, and abuse traffic.

UDP is not supported.

A sufficiently large Layer 7 attack can overwhelm a small VPS.

The service operator is responsible for abuse handling, acceptable-use
policies, user management, traffic monitoring, VPS security, domain security,
and credential management.

## Operational Recommendations

For production:

- Keep Ubuntu updated.
- Use SSH keys rather than password authentication.
- Keep port 9800 private.
- Do not expose the SQLite database.
- Do not commit tokens or passwords.
- Rotate compromised tokens immediately.
- Use strong admin credentials.
- Back up `/var/lib/tunnel`.
- Keep release binaries and checksums.
- Monitor system resources.
- Monitor `journalctl -u tunnel-server`.
- Use the VPS provider firewall together with UFW where practical.

## License

MIT License.

See [LICENSE](LICENSE).
