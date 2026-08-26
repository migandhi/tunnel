# Security Policy

## Supported Versions

Security fixes are provided for the latest stable release.

| Version | Supported |
|---|---|
| v1.0.x | Yes |
| Older versions | No |

## Reporting a Vulnerability

If you discover a security vulnerability in Tunnel, please report it privately rather than opening a public GitHub issue.

Please include:

- A description of the vulnerability
- Steps to reproduce it
- The affected version
- The potential impact
- Any relevant logs or proof-of-concept information

For security reports, contact:

**murtaza.i.gandhi@gmail.com**

Please do not publicly disclose the vulnerability until it has been reviewed and a fix or mitigation is available.

## Security Considerations

Tunnel is networking software that can expose local services to the public Internet.

Operators should:

- Keep the tunnel server and clients updated.
- Protect authentication tokens.
- Never commit tokens, passwords, private keys, certificates, databases, or `.env` files to the repository.
- Restrict access to the server administration interface.
- Use strong, unique administrative credentials.
- Monitor publicly exposed tunnels.
- Run the tunnel server with the least privileges necessary.

## Responsible Disclosure

We appreciate responsible disclosure of security issues and will make reasonable efforts to investigate reported vulnerabilities and provide appropriate fixes.