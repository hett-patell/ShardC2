.PHONY: all build server agent bruteforcer clean docker-up docker-down

BINARY_DIR=bin
GO=go

all: build

build: server agent bruteforcer

server:
	$(GO) build -o $(BINARY_DIR)/shardc2-server ./cmd/server

agent:
	$(GO) build -o $(BINARY_DIR)/shardc2-agent ./cmd/agent

bruteforcer:
	$(GO) build -o $(BINARY_DIR)/shardc2-brute ./cmd/bruteforcer

clean:
	rm -rf $(BINARY_DIR)

docker-up:
	docker compose up -d

docker-down:
	docker compose down

cross-compile:
	GOOS=linux GOARCH=amd64 $(GO) build -o $(BINARY_DIR)/agent-linux-amd64 ./cmd/agent
	GOOS=linux GOARCH=arm64 $(GO) build -o $(BINARY_DIR)/agent-linux-arm64 ./cmd/agent
	GOOS=linux GOARCH=arm GOARM=7 $(GO) build -o $(BINARY_DIR)/agent-linux-arm7 ./cmd/agent
	GOOS=windows GOARCH=amd64 $(GO) build -o $(BINARY_DIR)/agent-windows-amd64.exe ./cmd/agent
	GOOS=darwin GOARCH=arm64 $(GO) build -o $(BINARY_DIR)/agent-darwin-arm64 ./cmd/agent
