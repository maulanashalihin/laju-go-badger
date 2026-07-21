APP_NAME   := laju-go
BINARY     := $(APP_NAME)
VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT     ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
LDFLAGS    := -ldflags="-X main.Version=$(VERSION) -X main.Commit=$(COMMIT)"

# Windows: pake WSL (wsl make build) atau winget install GnuWin32.Make
ifeq ($(OS),Windows_NT)
BINARY := $(APP_NAME).exe
endif

.PHONY: build build-go build-linux verify test lint generate templ clean docker version db-reset setup

setup:
	@echo "==> Checking prerequisites..."
	@command -v go >/dev/null 2>&1 || { echo "Error: Go is not installed. Install Go 1.26+ from https://go.dev/dl/"; exit 1; }
	@command -v node >/dev/null 2>&1 || { echo "Error: Node.js is not installed. Install Node 22+ from https://nodejs.org/"; exit 1; }
	@command -v npm >/dev/null 2>&1 || { echo "Error: npm is not installed (comes with Node.js)"; exit 1; }
	@command -v templ >/dev/null 2>&1 || { echo "Warning: 'templ' CLI not found. Install with: go install github.com/a-h/templ/cmd/templ@latest"; }
	@echo "    Go:      $$(go version)"
	@echo "    Node:    $$(node --version)"
	@echo "    npm:     $$(npm --version)"
	@echo ""
	@echo "==> Copying .env (if not present)..."
	@if [ ! -f .env ]; then cp .env.example .env && echo "    Created .env from .env.example"; else echo "    .env already exists, skipping"; fi
	@echo ""
	@echo "==> Installing Go dependencies..."
	@go mod download
	@echo ""
	@echo "==> Installing Node dependencies..."
	@npm install
	@echo ""
	@echo "==> Generating templ files..."
	@if command -v templ >/dev/null 2>&1; then templ generate; else echo "    Skipped (templ CLI not installed)"; fi
	@echo ""
	@echo "==> Setup complete!"
	@echo ""
	@echo "Next steps:"
	@echo "  1. Edit .env (set SESSION_SECRET, GOOGLE_CLIENT_ID, etc.)"
	@echo "  2. Run:  npm run dev:all"
	@echo "  3. Visit http://localhost:8080"

build: vite-build go-build

build-go: go-build

build-linux:
	GOOS=linux GOARCH=amd64 go build -trimpath $(LDFLAGS) -o $(BINARY) ./cmd/laju-go

go-build:
	go build -trimpath $(LDFLAGS) -o $(BINARY) ./cmd/laju-go

vite-build:
	npm run build

verify:
	npm run verify
	go test ./...

test:
	go test ./...

lint:
	golangci-lint run ./...

generate: templ

templ:
	templ generate

db-reset:
	rm -rf ./data/badger

clean:
	rm -rf $(BINARY) tmp/ dist/ data/badger

docker:
	docker build -t $(APP_NAME) .

version:
	@echo "$(VERSION) (commit: $(COMMIT))"
