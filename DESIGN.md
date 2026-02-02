# credwrap

**Secure credential injection for AI agents**

## Problem

AI agents with shell access can read credentials from:
- Environment variables (`env`, `echo $API_KEY`)
- Config files (`cat .env`, `cat ~/.config/*/credentials`)
- Keychain queries (`security find-generic-password`, `op read`)

Even "secure" solutions like 1Password CLI or HashiCorp Vault are vulnerable because the agent can query them directly.

## Solution

A privilege-separated credential wrapper that:
1. Stores credentials in a location the agent user **cannot read**
2. Exposes only an **allowlisted set of commands**
3. Injects credentials transparently at execution time
4. Never reveals credential values to the calling process

## Architecture

```
┌────────────────────────────────────────────────────────┐
│  Agent (clawd user)                                    │
│  - Calls: credwrap <tool> [args...]                    │
│  - Cannot read /var/lib/credwrap/*                     │
│  - Cannot query credentials directly                   │
└────────────────────────────────────────────────────────┘
                         │
                         ▼
┌────────────────────────────────────────────────────────┐
│  credwrap binary (setuid credwrap OR sudo rules)       │
│  - Validates tool is in allowlist                      │
│  - Loads credential config                             │
│  - Injects env vars / flags                            │
│  - Executes tool, returns output                       │
│  - Scrubs credentials from output (optional)           │
└────────────────────────────────────────────────────────┘
                         │
                         ▼
┌────────────────────────────────────────────────────────┐
│  /var/lib/credwrap/                                    │
│  ├── config.yaml      (tool definitions)               │
│  ├── credentials.enc  (encrypted at rest)              │
│  └── keyfile          (decryption key, mode 400)       │
│  Owned by credwrap:credwrap, mode 700                  │
└────────────────────────────────────────────────────────┘
```

## Configuration

### Tool allowlist (`config.yaml`)

```yaml
tools:
  gog:
    path: /home/clawd/.npm-global/bin/gog
    credentials:
      - env: GOG_KEYRING_PASSWORD
        secret: gog-keyring-password
    pass_through_args: true
    
  bird:
    path: /home/clawd/.npm-global/bin/bird
    credentials:
      - env: BIRD_COOKIES
        secret: bird-cookies
    pass_through_args: true
    
  ssh:
    path: /usr/bin/ssh
    credentials:
      - type: ssh-agent
        key: default-ssh-key
    # No pass_through_args - only allow specific hosts?
    allowed_args_pattern: "^[a-z0-9.-]+$"  # hostname only
    
  curl-anthropic:
    path: /usr/bin/curl
    credentials:
      - header: "x-api-key"
        secret: anthropic-api-key
      - header: "anthropic-version"
        value: "2023-06-01"  # static, not secret
    # Locked to specific URL pattern
    allowed_args_pattern: "^https://api\\.anthropic\\.com/"
```

### Credentials store (`credentials.enc`)

Encrypted YAML/JSON with secret values:

```yaml
gog-keyring-password: "actual-password-here"
bird-cookies: "cookie-string-here"
anthropic-api-key: "sk-ant-..."
default-ssh-key: |
  -----BEGIN OPENSSH PRIVATE KEY-----
  ...
  -----END OPENSSH PRIVATE KEY-----
```

## Usage

```bash
# Instead of:
GOG_KEYRING_PASSWORD=secret gog gmail search 'is:unread'

# Agent calls:
credwrap gog gmail search 'is:unread'

# Instead of:
ssh -i ~/.ssh/id_rsa user@host

# Agent calls:
credwrap ssh user@host
```

## Security Properties

### What the agent CAN do:
- Execute allowlisted commands via credwrap
- Pass arguments to those commands
- Receive command output

### What the agent CANNOT do:
- Read credential files directly (Unix permissions)
- Query credentials via credwrap (no `credwrap get-secret`)
- Execute arbitrary commands through credwrap
- Access credentials in memory (separate process)

## Implementation: TCP Service Architecture

After consideration of cross-platform support (Linux + macOS) and multi-machine setups, the chosen architecture is a **TCP client-server model**.

### Components

| Component | Role |
|-----------|------|
| `credwrap` | CLI client, connects to server, streams I/O |
| `credwrap-server` | Daemon, holds secrets in memory, executes tools, audit logs |
| `config.yaml` | Tool allowlist, server bind address, auth settings |
| `credentials.enc` | Encrypted secrets (age), decrypted on server startup |

### Why TCP (not Unix socket only)?

1. **Multi-machine isolation**: Credentials can live on a physically separate machine (e.g., Mac Mini), agent runs elsewhere (e.g., beelink2)
2. **Tailscale integration**: Service binds to Tailscale IP, gets node identity for free
3. **Cross-platform**: TCP works identically on Linux and macOS
4. **Can still do local**: Bind to 127.0.0.1 for single-machine setups

### Protocol

Simple newline-delimited JSON over TCP with streaming support.

**Exec request:**
```json
{
  "type": "exec",
  "token": "abc123",
  "tool": "gog",
  "args": ["gmail", "search", "is:unread"],
  "env": {"OPTIONAL": "extra-env"}
}
```

**Server responses (streamed):**
```json
{"type": "started", "pid": 1234}
{"type": "stdout", "data": "line of output..."}
{"type": "stderr", "data": "warning..."}
{"type": "exit", "code": 0}
```

**Stdin (client to server, during exec):**
```json
{"type": "stdin", "data": "user input\n"}
{"type": "stdin_close"}
```

### Authentication

**Option 1: Simple token** (recommended for local/Tailscale)
- Token in config file on client side
- If service only listens on localhost or Tailscale, exfiltrated token is useless
- Simple, low overhead

**Option 2: Tailscale identity**
- Server checks Tailscale peer identity via local API
- Allowlist of authorized Tailscale node IDs
- No token needed — network *is* the auth

**Option 3: Both**
- Tailscale identity for node-level auth
- Token for additional per-agent isolation (if multiple agents on same node)

### Server Startup

```bash
# Interactive: password unlocks credentials
credwrap-server --config config.yaml
Password: ********
Loaded 5 credentials, listening on 127.0.0.1:9876

# Unattended: keyfile (must be permission-protected)
credwrap-server --config config.yaml --keyfile /secure/path/key

# macOS: use Keychain for master key
credwrap-server --config config.yaml --keychain "credwrap-master"
```

### Audit Logging

Every request logged:
```json
{
  "ts": "2026-02-02T03:45:00Z",
  "client": "100.100.132.21",
  "tailscale_node": "beelink2",
  "tool": "gog",
  "args": ["gmail", "search", "is:unread"],
  "exit_code": 0,
  "duration_ms": 234
}
```

### Deployment Modes

| Mode | Bind address | Auth | Use case |
|------|--------------|------|----------|
| **Local** | 127.0.0.1:9876 | Token | Single machine, agent and server same host |
| **Tailscale** | 100.x.x.x:9876 | Token or Tailscale ID | Multi-machine, credentials isolated |
| **LAN** | 0.0.0.0:9876 | Token (careful!) | Trusted home network |

## SSH Special Handling

For SSH, we have options:

### A: Inject key via temp file
- Write key to temp file (mode 600, in credwrap-owned dir)
- Run `ssh -i /tmp/credwrap-xyz/key user@host`
- Delete temp file after
- Risk: Key exists on disk briefly

### B: Use ssh-agent
- credwrap spawns ssh-agent, adds key
- Sets SSH_AUTH_SOCK for the ssh command
- Agent dies after command completes
- Better: Key never on disk

### C: SSH certificates (advanced)
- credwrap signs short-lived SSH certificates
- User has CA key, signs cert valid for 60 seconds
- Best security, but requires SSH CA setup

## Output Scrubbing (Optional)

Some tools might echo credentials in error messages or debug output. Options:
- Regex scrub known secret patterns from stdout/stderr
- Replace with `[REDACTED]`
- Configurable per-tool

## Future Enhancements

- **Audit logging**: Log all credwrap invocations
- **Rate limiting**: Prevent rapid-fire credential use
- **Time-based access**: Credentials only available during certain hours
- **Approval workflow**: Some commands require human approval
- **Remote vault backend**: Pull credentials from 1Password/Vault instead of local file

## Open Questions

1. **Language choice**: Go (easy distribution), Rust (more secure), Python (faster to prototype)?
2. **Encryption**: Age, SOPS, or just rely on Unix permissions?
3. **How to handle credential rotation?** (Admin updates credentials.enc)
4. **Should this be an OpenClaw built-in or standalone tool?**

## Next Steps

1. [ ] Prototype in Python to validate the concept
2. [ ] Test with gog, bird, ssh
3. [ ] Rewrite in Go/Rust for production
4. [ ] Package for easy installation
5. [ ] Contribute to OpenClaw / publish as standalone
