VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)
export CGO_ENABLED=0

.PHONY: all frontend build test test-go typecheck dev-backend dev-frontend dist clean

all: build

frontend:
	cd frontend && npm ci && npm run build

build: frontend
	go build -ldflags "$(LDFLAGS)" -o bin/wakepilot ./cmd/server

test: test-go typecheck

test-go:
	go vet ./...
	go test -count=1 ./...

typecheck:
	cd frontend && npm run typecheck

dev-backend:
	go run ./cmd/server -config configs/config.example.yaml

dev-frontend:
	cd frontend && npm run dev

# Cross-compile standalone binaries (needs the frontend built first).
dist: frontend
	./scripts/build-all.sh "$(VERSION)"

clean:
	rm -rf bin dist-bin frontend/dist/* && touch frontend/dist/.gitkeep
