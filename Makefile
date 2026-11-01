.PHONY: build web run test vet fmt docker clean help

APP_NAME := wireguardpanel
IMAGE    := ghcr.io/ndo4me/wireguard-server-panel

## build: Build the Go binary (CGO is required for SQLite)
build:
	CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o $(APP_NAME) ./cmd/api

## web: Build the frontend into web/dist
web:
	cd web && npm ci && npm run build

## run: Build everything and run locally (needs root for WireGuard/Tailscale)
run: web build
	./$(APP_NAME)

## test: Run Go tests
test:
	go test ./...

## vet: Static checks
vet:
	go vet ./...

## fmt: Format Go code
fmt:
	gofmt -w cmd internal pkg

## docker: Build the Docker image locally
docker:
	docker build -t $(IMAGE):local .

## clean: Remove build artifacts
clean:
	rm -rf $(APP_NAME) web/dist web/node_modules coverage.out

## help: List targets
help:
	@grep -E '^## ' Makefile | sed 's/^## //'
