# ═══════════════════════════════════════════════════════════════════════════════
# OpenClause — Makefile
# ═══════════════════════════════════════════════════════════════════════════════

.PHONY: dev dev-down test policy-test lint build clean migrate help

# Default env file
ENV_FILE ?= .env

# ── Development ───────────────────────────────────────────────────────────────

## Start all services locally via Docker Compose
dev:
	@echo ">>> Starting OpenClause stack..."
	@cp -n .env.example .env 2>/dev/null || true
	docker compose -f deploy/docker-compose.yml up --build -d
	@echo ">>> Waiting for postgres..."
	@sleep 3
	@$(MAKE) migrate
	@echo ""
	@echo "✓ Gateway:    http://localhost:8080/healthz"
	@echo "✓ Approvals:  http://localhost:8081/healthz"
	@echo "✓ Slack:      http://localhost:8082/healthz"
	@echo "✓ Jira:       http://localhost:8083/healthz"
	@echo "✓ OPA:        http://localhost:8181/health"
	@echo "✓ MinIO:      http://localhost:9001"

## Stop all services
dev-down:
	docker compose -f deploy/docker-compose.yml down -v

## View logs
logs:
	docker compose -f deploy/docker-compose.yml logs -f

# ── Database ──────────────────────────────────────────────────────────────────

## Run database migrations
migrate:
	@echo ">>> Running migrations..."
	PGPASSWORD=$$(grep POSTGRES_PASSWORD $(ENV_FILE) | cut -d= -f2) \
	psql -h localhost -p 5432 \
		-U $$(grep POSTGRES_USER $(ENV_FILE) | cut -d= -f2) \
		-d $$(grep POSTGRES_DB $(ENV_FILE) | cut -d= -f2) \
		-f migrations/001_initial.sql \
	|| docker compose -f deploy/docker-compose.yml exec -T postgres \
		psql -U openclause -d openclause -f /dev/stdin < migrations/001_initial.sql
	@echo "✓ Migrations complete"

# ── Testing ───────────────────────────────────────────────────────────────────

## Run all tests (Go + Policy)
test: policy-test go-test

## Run Go unit tests
go-test:
	@echo ">>> Running Go tests..."
	go test ./... -v -count=1

## Run OPA policy tests
policy-test:
	@echo ">>> Running policy tests..."
	opa test policy/bundles/v0/ policy/tests/ -v

## Lint Go code
lint:
	@echo ">>> Linting Go code..."
	golangci-lint run ./...

# ── Build ─────────────────────────────────────────────────────────────────────

## Build all Go binaries locally
build:
	@echo ">>> Building binaries..."
	CGO_ENABLED=0 go build -o bin/gateway ./cmd/gateway
	CGO_ENABLED=0 go build -o bin/approvals ./cmd/approvals
	CGO_ENABLED=0 go build -o bin/connector-slack ./cmd/connector-slack
	CGO_ENABLED=0 go build -o bin/connector-jira ./cmd/connector-jira
	@echo "✓ Binaries in bin/"

## Build Docker images
docker-build:
	docker build --build-arg SERVICE_NAME=gateway -t oc-gateway .
	docker build --build-arg SERVICE_NAME=approvals -t oc-approvals .
	docker build --build-arg SERVICE_NAME=connector-slack -t oc-connector-slack .
	docker build --build-arg SERVICE_NAME=connector-jira -t oc-connector-jira .

## Clean build artifacts
clean:
	rm -rf bin/
	docker compose -f deploy/docker-compose.yml down -v --rmi local 2>/dev/null || true

# ── Help ──────────────────────────────────────────────────────────────────────

## Show this help
help:
	@echo "OpenClause"
	@echo ""
	@echo "Usage: make <target>"
	@echo ""
	@echo "Targets:"
	@echo "  dev           Start all services locally (Docker Compose)"
	@echo "  dev-down      Stop and remove all services"
	@echo "  logs          Tail logs from all services"
	@echo "  migrate       Run database migrations"
	@echo "  test          Run all tests (Go + policy)"
	@echo "  go-test       Run Go unit tests"
	@echo "  policy-test   Run OPA policy tests"
	@echo "  lint          Lint Go code"
	@echo "  build         Build Go binaries locally"
	@echo "  docker-build  Build Docker images"
	@echo "  clean         Clean build artifacts and containers"
