# credwrap

**Secure credential injection for AI agents**

credwrap is a client-server tool that allows AI agents to use credentials without ever seeing them. The server holds secrets in memory and injects them into pre-approved commands, while the agent only interacts through an allowlisted interface.

## The Problem

AI agents with shell access can read credentials from:
- Environment variables (`env`, `echo $API_KEY`)
- Config files (`cat .env`, `cat ~/.config/*/credentials`)
- Keychain queries (`security find-generic-password`, `op read`)

Even "secure" solutions like 1Password CLI or HashiCorp Vault are vulnerable because the agent can query them directly.

## The Solution

credwrap uses privilege separation:

1. **The server** runs separately (different process, optionally different machine)
2. **Credentials** are encrypted at rest, decrypted only in server memory
3. **Agents** can only execute pre-approved tools via the client
4. **No credential values** ever appear in the agent's context

```
┌─────────────────────┐         ┌─────────────────────┐
│  Agent              │         │  credwrap-server    │
│  (can't see creds)  │◄──TCP──►│  (holds secrets)    │
└─────────────────────┘         └─────────────────────┘
```

## Installation

```bash
go install github.com/openclaw/credwrap/cmd/credwrap@latest
go install github.com/openclaw/credwrap/cmd/credwrap-server@latest
```

Or build from source:

```bash
git clone https://github.com/openclaw/credwrap
cd credwrap
go build -o credwrap ./cmd/credwrap
go build -o credwrap-server ./cmd/credwrap-server
```

## Quick Start

### 1. Create server config

```yaml
# /etc/credwrap/config.yaml
server:
  listen: "127.0.0.1:9876"
  audit: "/var/log/credwrap/audit.log"

auth:
  tokens:
    - "generate-a-secure-token"

tools:
  gog:
    path: /usr/local/bin/gog
    credentials:
      - env: GOG_KEYRING_PASSWORD
        secret: gog-password
    pass_args: true
```

### 2. Create credentials file

```yaml
# /etc/credwrap/credentials.yaml (chmod 600!)
gog-password: "actual-password-here"
```

### 3. Start the server

```bash
credwrap-server --config /etc/credwrap/config.yaml \
                --credentials /etc/credwrap/credentials.yaml
```

### 4. Configure the client

```yaml
# ~/.credwrap.yaml
server: "127.0.0.1:9876"
token: "generate-a-secure-token"
```

### 5. Use it

```bash
# Instead of:
GOG_KEYRING_PASSWORD=secret gog gmail search 'is:unread'

# Agent calls:
credwrap gog gmail search 'is:unread'
```

## Multi-Machine Setup

For maximum security, run the server on a separate machine (e.g., over Tailscale):

```yaml
# Server on mac-mini
server:
  listen: "100.100.132.22:9876"  # Tailscale IP
```

```yaml
# Client on beelink2
server: "100.100.132.22:9876"
token: "your-token"
```

The agent on beelink2 can use credentials, but they never leave mac-mini.

## Security Model

### What the agent CAN do:
- Execute allowlisted commands via credwrap
- Pass arguments to those commands
- Receive command output

### What the agent CANNOT do:
- Read credential files (wrong machine or permissions)
- Query credentials via credwrap (no query interface)
- Execute arbitrary commands (allowlist only)
- Access credentials in memory (separate process)

## Audit Logging

Every command is logged:

```json
{
  "ts": "2026-02-02T03:45:00Z",
  "client": "100.100.132.21:54321",
  "tool": "gog",
  "args": ["gmail", "search", "is:unread"],
  "exit_code": 0,
  "duration_ms": 234,
  "status": "ok"
}
```

## Encryption (Coming Soon)

Encrypt credentials with age:

```bash
age -p credentials.yaml > credentials.enc
```

Start server with password:

```bash
credwrap-server --config config.yaml \
                --credentials credentials.enc \
                --encrypted
```

## License

MIT

## Contributing

Issues and PRs welcome at https://github.com/openclaw/credwrap
