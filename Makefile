.PHONY: all build server agent bruteforcer clean test docker-up docker-down generate-cert agent-deploy cross-compile generate-payload-key

BINARY_DIR=bin
GO=go
LDFLAGS=-s -w
SERVER_URL ?= https://localhost:8443
IMPLANT_KEY ?= changeme
PAYLOAD_KEY ?=
KILL_DATE ?=

all: build

build: server agent bruteforcer

server:
	$(GO) build -o $(BINARY_DIR)/shardc2-server ./cmd/server

agent:
	$(GO) build -ldflags "$(LDFLAGS)" -o $(BINARY_DIR)/shardc2-agent ./cmd/agent

bruteforcer:
	$(GO) build -ldflags "$(LDFLAGS)" -o $(BINARY_DIR)/shardc2-brute ./cmd/bruteforcer

agent-deploy:
	$(GO) build -ldflags "$(LDFLAGS) -X main.buildServerURL=$(SERVER_URL) -X main.buildImplantKey=$(IMPLANT_KEY) -X main.buildPayloadKey=$(PAYLOAD_KEY) -X main.buildKillDate=$(KILL_DATE)" -o $(BINARY_DIR)/agent-deploy ./cmd/agent

generate-payload-key:
	@head -c 32 /dev/urandom | xxd -p -c 64

clean:
	rm -rf $(BINARY_DIR)

test:
	$(GO) test ./... -race -count=1

docker-up:
	docker compose up -d

docker-down:
	docker compose down

generate-cert:
	$(GO) run ./cmd/server --generate-cert

AGENT_LDFLAGS=$(LDFLAGS) -X main.buildServerURL=$(SERVER_URL) -X main.buildImplantKey=$(IMPLANT_KEY) -X main.buildPayloadKey=$(PAYLOAD_KEY) -X main.buildKillDate=$(KILL_DATE)

cross-compile:
	GOOS=linux GOARCH=amd64 $(GO) build -ldflags "$(AGENT_LDFLAGS)" -o $(BINARY_DIR)/agent-linux-amd64 ./cmd/agent
	GOOS=linux GOARCH=arm64 $(GO) build -ldflags "$(AGENT_LDFLAGS)" -o $(BINARY_DIR)/agent-linux-arm64 ./cmd/agent
	GOOS=linux GOARCH=arm GOARM=7 $(GO) build -ldflags "$(AGENT_LDFLAGS)" -o $(BINARY_DIR)/agent-linux-arm7 ./cmd/agent
	GOOS=windows GOARCH=amd64 $(GO) build -ldflags "$(AGENT_LDFLAGS)" -o $(BINARY_DIR)/agent-windows-amd64.exe ./cmd/agent
	GOOS=darwin GOARCH=arm64 $(GO) build -ldflags "$(AGENT_LDFLAGS)" -o $(BINARY_DIR)/agent-darwin-arm64 ./cmd/agent
