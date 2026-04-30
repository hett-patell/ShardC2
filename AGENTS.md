# AGENTS.md

## Setup
- Start DB: `make docker-up` (PostgreSQL on 5432, Redis on 6379 with passwords)
- Install deps: `go mod tidy`
- Build all: `make build` (creates bin/ with server, agent, bruteforcer)

## Building
- Server: `make server` → bin/shardc2-server
- Agent: `make agent` → bin/shardc2-agent
- Cross-compile agents: `make cross-compile` (Linux/Windows/macOS binaries)

## Testing
- All tests: `go test ./...`
- Single package: `go test ./internal/agent`
- Single test: `go test -run TestBeacon`

## Architecture
- Entry points: cmd/server, cmd/agent, cmd/bruteforcer
- Packages: internal/ (server/agent logic), pkg/models (shared structs)
- DB: PostgreSQL via migrations/ SQL files
- Web: Fiber framework for API

## Conventions
- Commit messages: "feat:", "fix:", "chore:"
- DB passwords: shardc2_secret (PG), shardc2_redis (Redis)
- Agent binaries: Deploy to targets for C2 operations

## Constraints
- Use only for authorized red teaming; avoid production or malicious deployment
- Agents include persistence (cron) and command execution - handle with care