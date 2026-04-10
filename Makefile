.DEFAULT_GOAL := help
.PHONY: build run test coverage vet clean install setup help

BINARY     := mcp-cli-gateway
BUILD      := ./cmd/mcp-cli-gateway
OUT        := ./$(BINARY)

# ── Development ──────────────────────────────────────────────

run: build ## Run the server directly
	$(OUT)

# ── Build ────────────────────────────────────────────────────

build: ## Build the binary
	go build -o $(OUT) $(BUILD)

install: ## Install binary to GOPATH/bin
	go install $(BUILD)

# ── Testing ──────────────────────────────────────────────────

test: ## Run all tests
	go test ./... -count=1

coverage: ## Run tests with coverage report
	go test ./... -count=1 -coverprofile=coverage.out
	go tool cover -func=coverage.out
	@echo "---"
	@go tool cover -func=coverage.out | tail -1

coverage-html: coverage ## Run tests with HTML coverage report
	go tool cover -html=coverage.out -o coverage.html
	@echo "Open coverage.html in your browser"

# ── Quality ──────────────────────────────────────────────────

vet: ## Run go vet
	go vet ./...

check: vet test ## Run all checks (vet + test)

# ── Setup ────────────────────────────────────────────────────

setup: ## First-time setup: copy .env, tidy deps
	@test -f .env || cp .env.example .env && echo "Created .env from .env.example"
	go mod tidy
	@mkdir -p data
	@echo "Ready. Run 'make run' to start."


# ── Cleanup ──────────────────────────────────────────────────

clean: ## Remove build artifacts
	rm -f $(BINARY) coverage.out coverage.html

# ── Help ─────────────────────────────────────────────────────

help: ## Show this help
	@echo "Usage: make [target]"
	@echo ""
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-16s %s\n", $$1, $$2}' $(MAKEFILE_LIST)
