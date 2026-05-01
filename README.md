# ShardC2

A modular, policy-aware command and control framework built in Go for authorized red team operations and security research.

<img width="1672" height="941" alt="ShardC2 Dashboard" src="https://github.com/user-attachments/assets/b5498fda-a45c-4e7e-b57a-f0adde39cab0" />

## Features

**Operator Interface**
- Web dashboard with real-time WebSocket updates
- Terminal with multi-implant command dispatch
- Campaign management with live progress tracking
- Credential vault with masked passwords and audited reveal
- Remote file browser
- Safety policy status panel

**Agent**
- Cross-platform (Linux, macOS, Windows) with ARM/AMD64 support
- AES-256-GCM encrypted + HMAC-signed payloads
- Malleable C2 profiles (default, CloudFront, WordPress)
- Sandbox/VM detection (process names, cgroup, MAC prefixes, disk size)
- Configurable beacon interval with jitter
- Kill date enforcement and session token refresh
- Persistence via cron with randomized filenames
- SOCKS5 proxy support

**Server**
- JWT-based RBAC (admin, operator, viewer) with bootstrap token
- Campaign engine with brute force, recon, exfil, persist, and custom task types
- Scope enforcement via safety policy (allowed/blocked CIDRs, safe mode)
- Full audit logging of operator actions
- Campaign dry-run validation before launch
- Dynamic agent build pipeline (cross-compile with garble obfuscation)
- Campaign evidence/report export
- Plugin system with signed manifests

**Security**
- Mandatory payload encryption when key is configured
- Per-agent session tokens with expiry
- Agent deduplication via fingerprint
- Rate limiting on login and agent routes
- Separate JWT and operator secrets
- TLS with explicit trust configuration

---

## Quick Start

### Prerequisites

- Go 1.21+
- Docker & Docker Compose
- PostgreSQL 15 (or use the included Docker setup)

### 1. Start Infrastructure

```bash
make docker-up
```

This starts PostgreSQL and Redis via Docker Compose.

### 2. Generate TLS Certificates

```bash
make generate-cert
```

### 3. Build

```bash
make build
```

Produces `bin/shardc2-server`, `bin/shardc2-agent`, and `bin/shardc2-brute`.

### 4. Start the Server

```bash
./bin/shardc2-server \
  --addr :8443 \
  --tls-cert server.crt \
  --tls-key server.key \
  --migrate
```

The server prints a generated bootstrap token on first run. Use it to create the initial admin operator.

### 5. Create Admin Operator

```bash
curl -sk -X POST \
  -H "Authorization: Bearer <bootstrap-token>" \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"your-password","role":"admin"}' \
  https://localhost:8443/api/v1/operators
```

### 6. Access Dashboard

Open `https://localhost:8443/dashboard/` and log in with your admin credentials.

---

## Architecture

```
shardc2/
├── cmd/
│   ├── server/          # C2 server entry point
│   ├── agent/           # Implant entry point
│   └── bruteforcer/     # Standalone brute force tool
├── internal/
│   ├── server/
│   │   ├── handlers/    # REST API handlers (bot, command, campaign, credential, ...)
│   │   ├── middleware/   # Auth (JWT, HMAC, implant), payload crypto
│   │   ├── engine/      # Campaign engine, supervisor, brute force
│   │   ├── audit/       # Audit event recorder
│   │   ├── builds/      # Dynamic agent build pipeline
│   │   └── report/      # Campaign evidence export
│   ├── agent/           # Agent core (beacon, evasion, persistence, proxy)
│   ├── database/        # PostgreSQL connection and migrations
│   └── testutil/        # Integration test helpers
├── pkg/
│   ├── crypto/          # AES-256-GCM encryption, HMAC signing
│   ├── policy/          # Safety policy engine (CIDR validation, safe mode)
│   ├── profiles/        # Malleable C2 profiles
│   ├── transport/       # Transport abstraction layer
│   ├── plugins/         # Plugin manifest validation
│   ├── client/          # Go operator SDK
│   └── models/          # Shared data models
├── migrations/          # PostgreSQL schema migrations (001-010)
├── web/dashboard/       # Operator web dashboard (HTML/CSS/JS)
├── docs/api/            # OpenAPI specification
└── docker-compose.yml   # PostgreSQL + Redis
```

---

## Server Configuration

| Flag | Env Variable | Default | Description |
|------|-------------|---------|-------------|
| `--addr` | | `:8443` | Listen address |
| `--db` | `SHARDC2_DB` | `postgres://...` | PostgreSQL connection string |
| `--migrate` | | `false` | Run migrations on startup |
| `--bootstrap-token` | `SHARDC2_BOOTSTRAP_TOKEN` | (generated) | Token for initial admin creation |
| `--implant-key` | `SHARDC2_IMPLANT_KEY` | (generated) | Agent authentication key |
| `--payload-key` | `SHARDC2_PAYLOAD_KEY` | | AES payload encryption key (hex, 32 bytes) |
| `--c2-url` | `SHARDC2_C2_URL` | | External URL for agent auto-deployment |
| `--tls-cert` | | | TLS certificate file |
| `--tls-key` | | | TLS private key file |
| `--profile` | | `default` | Malleable C2 profile |
| `--jwt-secret` | `SHARDC2_JWT_SECRET` | (generated) | JWT signing secret |
| `--policy-file` | `SHARDC2_POLICY_FILE` | | Safety policy JSON file |

---

## Agent Configuration

| Flag | Env Variable | Default | Description |
|------|-------------|---------|-------------|
| `--server` | `SHARDC2_SERVER` | (required) | C2 server URL |
| `--implant-key` | `SHARDC2_IMPLANT_KEY` | (required) | Implant auth key |
| `--payload-key` | `SHARDC2_PAYLOAD_KEY` | | Payload encryption key |
| `--interval` | | `5m` | Beacon interval |
| `--jitter` | | `60s` | Max beacon jitter |
| `--ca-cert` | | | CA certificate for TLS verification |
| `--kill-date` | `SHARDC2_KILL_DATE` | | Auto-terminate date (RFC3339) |
| `--insecure-tls-for-lab-only` | | `false` | Skip TLS verification |
| `--profile` | | `default` | Malleable C2 profile |
| `--daemon` | | `false` | Suppress banner output |
| `--ignore-sandbox` | | `false` | Skip sandbox detection |

### Deploying Agents

Build with embedded configuration:

```bash
make agent-deploy SERVER_URL=https://c2.example.com:8443 IMPLANT_KEY=your-key
```

Cross-compile for multiple platforms:

```bash
make cross-compile SERVER_URL=https://c2.example.com:8443 IMPLANT_KEY=your-key
```

Build obfuscated agents with [garble](https://github.com/burrowers/garble):

```bash
make agent-garble SERVER_URL=https://c2.example.com:8443 IMPLANT_KEY=your-key
```

---

## Safety Policy

ShardC2 ships with safe defaults. Create a policy file to control what operations are permitted:

```json
{
  "safe_mode": true,
  "allow_external_brute": false,
  "allow_auto_deploy": false,
  "allowed_cidrs": ["10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"],
  "allowed_hosts": ["lab-target.internal"],
  "blocked_cidrs": ["0.0.0.0/0"]
}
```

```bash
./bin/shardc2-server --policy-file policy.json ...
```

When `safe_mode` is enabled:
- Running campaigns are paused on server startup
- External brute force is blocked unless explicitly allowed
- Auto-deploy to cracked targets is blocked unless explicitly allowed
- Campaign targets are validated against allowed/blocked CIDRs

---

## Malleable Profiles

Profiles disguise C2 traffic as legitimate HTTP services:

| Profile | User-Agent | Path Pattern |
|---------|-----------|--------------|
| `default` | Mozilla/5.0 | `/api/v1/agent/*` |
| `cloudfront` | Amazon CloudFront | `/cdn-cgi/*` |
| `wordpress` | WordPress/6.4 | `/wp-json/wp/v2/*` |

Custom profiles can be defined in JSON and loaded with `--profile path/to/profile.json`.

---

## Campaign Types

| Type | Description |
|------|-------------|
| `recon` | System enumeration (sysinfo, network, users, software, cloud, containers, sensitive files) |
| `brute` | SSH brute force — lateral (via agents) or external (server-side) |
| `exfil` | File exfiltration from targets |
| `persist` | Deploy persistence mechanisms |
| `custom` | Arbitrary shell commands |

All campaigns support dry-run validation via `POST /api/v1/campaigns/validate` before launch.

---

## API

Full OpenAPI specification is available at [`docs/api/openapi.yaml`](docs/api/openapi.yaml).

Key endpoints:

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/v1/auth/login` | Operator login (returns JWT) |
| `GET` | `/api/v1/stats` | Server statistics |
| `GET` | `/api/v1/bots` | List registered agents |
| `POST` | `/api/v1/commands` | Send command to agent |
| `POST` | `/api/v1/commands/batch` | Send command to multiple agents |
| `POST` | `/api/v1/campaigns` | Create campaign |
| `POST` | `/api/v1/campaigns/:id/launch` | Launch campaign |
| `POST` | `/api/v1/campaigns/validate` | Dry-run validation |
| `GET` | `/api/v1/credentials` | List credentials (masked) |
| `GET` | `/api/v1/credentials/:id/reveal` | Reveal password (audited) |
| `POST` | `/api/v1/builds` | Request agent build |
| `GET` | `/api/v1/safety/status` | Safety policy status |
| `GET` | `/api/v1/plugins` | List installed plugins |

### Go SDK

```go
import "github.com/shardc2/shardc2/pkg/client"

c := client.New("https://c2.example.com:8443", jwtToken)

health, _ := c.Health()
stats, _ := c.Stats()
bots, _ := c.ListBots()
result, _ := c.ValidateCampaign(client.ValidateRequest{
    Type:   "brute",
    Config: `{"targets":["10.0.0.0/24"]}`,
})
```

---

## Database

ShardC2 uses PostgreSQL with incremental migrations:

| Migration | Purpose |
|-----------|---------|
| `001` | Core schema — bots, commands, credentials, campaigns |
| `002` | Bot authentication tokens |
| `003` | Campaign engine — campaign_bots, campaign_tasks |
| `004` | Agent deduplication (fingerprint), kill dates, indexes |
| `005` | Command timeout support |
| `006` | Operator accounts with RBAC |
| `007` | Audit events table |
| `008` | Agent identity — public keys, token expiry |
| `009` | Campaign run tracking |
| `010` | Agent build queue |

Migrations run automatically with `--migrate` flag.

---

## Development

```bash
# Start dependencies
make docker-up

# Run tests
make test

# Run with race detector
go test -race ./...

# Vet
go vet ./...

# Build everything
make build
```

### Integration Tests

Set `SHARDC2_TEST_DB` to run database-dependent tests:

```bash
export SHARDC2_TEST_DB="postgres://shardc2:shardc2_secret@localhost:5432/shardc2?sslmode=disable"
go test ./...
```

---

## Disclaimer

ShardC2 is intended for **authorized security testing, red team engagements, and educational purposes only**. Use only on systems you own or have explicit written permission to test. Unauthorized access to computer systems is illegal. The authors are not responsible for misuse.

---

## License

See [LICENSE](LICENSE) for details.
