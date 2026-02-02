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
# Server on credential-host (e.g., a separate secure machine)
server:
  listen: "100.64.1.50:9876"  # Tailscale IP
```

```yaml
# Client on agent-host
server: "100.64.1.50:9876"
token: "your-token"
```

The agent can use credentials, but they never leave the credential host.

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
  "client": "127.0.0.1:54321",
  "tool": "gog",
  "args": ["gmail", "search", "is:unread"],
  "exit_code": 0,
  "duration_ms": 234,
  "status": "ok"
}
```

## Encryption

Credentials are encrypted at rest using [age](https://github.com/FiloSottile/age). **Secrets never need to touch disk in plaintext.**

### Managing secrets (recommended method)

```bash
# Initialize a new encrypted credentials file
credwrap-server secrets init credentials.enc

# Add a secret (prompts for value - never written to disk in plaintext)
credwrap-server secrets add credentials.enc gog-password
credwrap-server secrets add credentials.enc api-key

# List secret names (not values)
credwrap-server secrets list credentials.enc

# Remove a secret
credwrap-server secrets rm credentials.enc old-secret
```

### Starting the server

**Interactive (prompted for password):**
```bash
credwrap-server --config config.yaml \
                --credentials credentials.enc \
                --encrypted
```

**Unattended with keyfile (for systemd/launchd):**
```bash
# Create a keyfile containing your password
echo "your-password" > /path/to/keyfile
chmod 600 /path/to/keyfile
chown credwrap:credwrap /path/to/keyfile  # if using separate user

# Start with keyfile
credwrap-server --config config.yaml \
                --credentials credentials.enc \
                --encrypted \
                --keyfile /path/to/keyfile
```

### Security tradeoffs

| Mode | Security | Convenience |
|------|----------|-------------|
| Password prompt | Highest - requires human | Must restart manually after reboot |
| Keyfile | Medium - file must be protected | Auto-start works |
| Plaintext creds | Lowest - relies only on permissions | Simplest setup |

For most users, **keyfile + file permissions** is a good balance.

## Deployment Options

### Option 1: Local user with encryption (simplest)

Run as your own user with encrypted credentials:

```bash
# Set up config directory
mkdir -p ~/.config/credwrap

# Create config (edit to add your tools)
cp examples/server-config.yaml ~/.config/credwrap/config.yaml

# Initialize encrypted credentials
credwrap-server secrets init ~/.config/credwrap/credentials.enc

# Add your secrets
credwrap-server secrets add ~/.config/credwrap/credentials.enc gog-password

# Start the server
credwrap-server --config ~/.config/credwrap/config.yaml \
                --credentials ~/.config/credwrap/credentials.enc \
                --encrypted
```

Security: relies on encryption (password required to start).

### Option 2: Separate user (Linux - maximum security)

```bash
# Download and run setup script
curl -sL https://raw.githubusercontent.com/akropp/credwrap/main/scripts/setup-server.sh | sudo bash

# Or with options
sudo ./scripts/setup-server.sh --user credwrap --port 9876 --bind 127.0.0.1
```

This creates:
- System user `credwrap` (no login shell)
- Config at `/etc/credwrap/config.yaml`
- Systemd service `credwrap.service`

**Systemd with encrypted credentials:**
```bash
# Add secrets
sudo -u credwrap credwrap-server secrets init /etc/credwrap/credentials.enc
sudo -u credwrap credwrap-server secrets add /etc/credwrap/credentials.enc gog-password

# Create keyfile for unattended startup
sudo bash -c 'echo "your-password" > /etc/credwrap/keyfile'
sudo chmod 600 /etc/credwrap/keyfile
sudo chown credwrap:credwrap /etc/credwrap/keyfile

# Edit service to use --encrypted --keyfile
sudo systemctl edit credwrap
# Add: ExecStart= line with --encrypted --keyfile /etc/credwrap/keyfile

sudo systemctl restart credwrap
```

**Systemd commands:**
```bash
sudo systemctl start credwrap
sudo systemctl stop credwrap
sudo systemctl status credwrap
sudo journalctl -u credwrap -f
```

### Option 3: macOS with launchd

```bash
# User service (runs as you)
./scripts/setup-macos.sh --user-service

# System service (runs as dedicated user)
sudo ./scripts/setup-macos.sh --system-service
```

**launchd with encrypted credentials:**
```bash
# Initialize credentials
credwrap-server secrets init ~/.config/credwrap/credentials.enc

# Add secrets
credwrap-server secrets add ~/.config/credwrap/credentials.enc api-key

# Create keyfile for unattended startup
echo "your-password" > ~/.config/credwrap/keyfile
chmod 600 ~/.config/credwrap/keyfile

# Load service
launchctl load ~/Library/LaunchAgents/com.credwrap.server.plist
```

**launchd commands:**
```bash
launchctl start com.credwrap.server
launchctl stop com.credwrap.server
launchctl list | grep credwrap
```

### Agent user setup

The agent user only needs the client config:

```yaml
# ~/.credwrap.yaml
server: "127.0.0.1:9876"
token: "your-token-from-server-config"
```

The agent can execute `credwrap <tool> [args]` but cannot:
- Read credential files (wrong user or encrypted)
- Query the server for credential values (no such API)
- Execute tools not in the allowlist

### Adding tools

**For standalone binaries (Go, Rust, etc.):**
```bash
sudo credwrap-server tools add /etc/credwrap/config.yaml gog ~/.local/bin/gog --env GOG_KEYRING_PASSWORD
```
This copies the binary to `/usr/local/bin` and updates the config.

**For interpreted tools (npm/pnpm, pip, etc.):**

These have dependencies that can't be simply copied. Options:

1. **Grant credwrap access to your user paths (simplest):**
   ```bash
   # Add credwrap to your group
   sudo usermod -aG $(whoami) credwrap
   
   # Make paths accessible
   chmod g+rx $HOME
   chmod -R g+rX $HOME/.local
   
   # Add tool without copying
   sudo credwrap-server tools add /etc/credwrap/config.yaml bird \
       ~/.local/share/pnpm/bird --no-copy --env BIRD_AUTH
   
   # Restart for group change to take effect
   sudo systemctl restart credwrap
   ```

2. **Install globally:**
   ```bash
   sudo npm install -g @steipete/bird
   sudo credwrap-server tools add /etc/credwrap/config.yaml bird \
       /usr/local/bin/bird --no-copy --env BIRD_AUTH
   ```

3. **Symlink (requires source path permissions):**
   ```bash
   sudo credwrap-server tools add /etc/credwrap/config.yaml bird \
       ~/.local/share/pnpm/bird --symlink --env BIRD_AUTH
   ```

**For system tools already in /usr/bin:**
```bash
sudo credwrap-server tools add /etc/credwrap/config.yaml gemini \
    /usr/bin/gemini --no-copy --env GEMINI_API_KEY
```

## License

MIT

## Contributing

Issues and PRs welcome at https://github.com/akropp/credwrap
