export GOCACHE ?= $(CURDIR)/.cache/go-build

.PHONY: setup dev build test smoke bench fmt e2e

setup:
	cd web && npm ci --cache ../.cache/npm --no-fund

dev:
	bash scripts/dev.sh

build:
	cd web && npm run build
	go build -o bin/evonardy ./cmd/evonardy

test:
	@test -z "$$(gofmt -l cmd internal)" || (gofmt -l cmd internal; exit 1)
	go vet ./...
	go test ./...
	go test -race ./...
	cd web && npm run typecheck && npm test && npm run build

smoke:
	go run ./cmd/evonardy simulate --config configs/baseline-smoke.json --data-dir .cache/smoke
	go run ./cmd/evonardy simulate --config configs/truncation-smoke.json --data-dir .cache/smoke
	go run ./cmd/evonardy replay --dir .cache/smoke/replays

bench:
	go test ./internal/game ./internal/features ./internal/arena -run '^$$' -bench . -benchmem -benchtime=200ms

e2e: build
	cd web && npm run e2e

fmt:
	gofmt -w cmd internal
