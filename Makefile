.PHONY: help postgres-start postgres-stop postgres-logs build test clean
.PHONY: migrate-workflow-up migrate-workflow-down migrate-workflow-status
.PHONY: run-go-worker run-python-worker dev-workers dev-all

# Load .env file if it exists
-include .env
export

# PostgreSQL configuration
POSTGRES_CONTAINER ?= pipeline-postgres
POSTGRES_IMAGE ?= postgres:17
POSTGRES_USER ?= pipeline
POSTGRES_PASSWORD ?= pwd
POSTGRES_DB ?= pipeline
POSTGRES_PORT ?= 5434

# Database URLs
DBOS_SYSTEM_DATABASE_URL ?= postgres://$(POSTGRES_USER):$(POSTGRES_PASSWORD)@localhost:$(POSTGRES_PORT)/$(POSTGRES_DB)?sslmode=disable
WORKFLOW_DATABASE_URL ?= postgres://$(POSTGRES_USER):$(POSTGRES_PASSWORD)@localhost:$(POSTGRES_PORT)/$(POSTGRES_DB)?sslmode=disable&search_path=workflow

# Content API (defaults to pas-server in dev mode)
CONTENT_API_URL ?= http://localhost:8080

# Worker configuration
WORKER_HTTP_ADDR ?= :8081
DBOS_APPLICATION_VERSION ?= content-pipeline-v1
DBOS_QUEUE_CONCURRENCY ?= 4

# Python worker
PYTHON_WORKER_PORT ?= 8082

help:
	@echo "Simple Content Pipeline - Local Development"
	@echo "==========================================="
	@echo ""
	@echo "Database:"
	@echo "  postgres-start          - Start PostgreSQL container"
	@echo "  postgres-stop           - Stop PostgreSQL container"
	@echo "  postgres-logs           - View PostgreSQL logs"
	@echo ""
	@echo "Workflow Migrations:"
	@echo "  migrate-workflow-up     - Apply workflow schema migrations"
	@echo "  migrate-workflow-down   - Rollback workflow migration"
	@echo "  migrate-workflow-status - Show workflow migration status"
	@echo ""
	@echo "Build:"
	@echo "  build                   - Build Go worker binaries"
	@echo "  test                    - Run Go tests"
	@echo "  clean                   - Clean build artifacts"
	@echo ""
	@echo "Development:"
	@echo "  run-go-worker           - Run Go worker (thumbnails)"
	@echo "  run-python-worker       - Run Python ML worker (OCR, YOLO)"
	@echo "  dev-workers             - Run both workers in parallel"
	@echo "  dev-all                 - Start database + run migrations + run workers"
	@echo ""
	@echo "Configuration:"
	@echo "  Copy .env.example to .env and customize for your setup"

# Database Management
postgres-start:
	@echo "Starting PostgreSQL container..."
	@podman run -d \
		--name $(POSTGRES_CONTAINER) \
		-e POSTGRES_USER=$(POSTGRES_USER) \
		-e POSTGRES_PASSWORD=$(POSTGRES_PASSWORD) \
		-e POSTGRES_DB=$(POSTGRES_DB) \
		-p $(POSTGRES_PORT):5432 \
		$(POSTGRES_IMAGE)
	@echo "✓ PostgreSQL started on port $(POSTGRES_PORT)"
	@echo "  Connection: postgresql://$(POSTGRES_USER):***@localhost:$(POSTGRES_PORT)/$(POSTGRES_DB)"

postgres-stop:
	@echo "Stopping PostgreSQL container..."
	@podman stop $(POSTGRES_CONTAINER) 2>/dev/null || true
	@podman rm $(POSTGRES_CONTAINER) 2>/dev/null || true
	@echo "✓ PostgreSQL stopped"

postgres-logs:
	@podman logs -f $(POSTGRES_CONTAINER)

# Workflow Migrations
migrate-workflow-up:
	@echo "Applying workflow migrations..."
	@cd ../simple-workflow && make migrate-up \
		DB_HOST=localhost \
		DB_PORT=$(POSTGRES_PORT) \
		DB_USER=$(POSTGRES_USER) \
		DB_PASSWORD=$(POSTGRES_PASSWORD) \
		DB_NAME=$(POSTGRES_DB)
	@echo "✓ Workflow migrations applied"

migrate-workflow-down:
	@echo "Rolling back workflow migration..."
	@cd ../simple-workflow && make migrate-down \
		DB_HOST=localhost \
		DB_PORT=$(POSTGRES_PORT) \
		DB_USER=$(POSTGRES_USER) \
		DB_PASSWORD=$(POSTGRES_PASSWORD) \
		DB_NAME=$(POSTGRES_DB)
	@echo "✓ Workflow migration rolled back"

migrate-workflow-status:
	@echo "Workflow migration status:"
	@cd ../simple-workflow && make migrate-status \
		DB_HOST=localhost \
		DB_PORT=$(POSTGRES_PORT) \
		DB_USER=$(POSTGRES_USER) \
		DB_PASSWORD=$(POSTGRES_PASSWORD) \
		DB_NAME=$(POSTGRES_DB)

# Build
build:
	@echo "Building pipeline-worker..."
	@go build -o pipeline-worker ./cmd/pipeline-worker
	@echo "✓ Build complete: pipeline-worker"

# Tests
test:
	@echo "Running Go tests..."
	@go test ./...
	@echo "✓ Tests passed"

# Clean
clean:
	@echo "Cleaning build artifacts..."
	@rm -f pipeline-worker pipeline-standalone
	@echo "✓ Clean complete"

# Development - Run Workers
run-go-worker:
	@echo "Starting Go worker (thumbnails)..."
	@echo "  DBOS URL: $(DBOS_SYSTEM_DATABASE_URL)"
	@echo "  Workflow URL: $(WORKFLOW_DATABASE_URL)"
	@echo "  Content API: $(CONTENT_API_URL)"
	@echo "  HTTP: $(WORKER_HTTP_ADDR)"
	@DBOS_SYSTEM_DATABASE_URL=$(DBOS_SYSTEM_DATABASE_URL) \
	 WORKFLOW_DATABASE_URL=$(WORKFLOW_DATABASE_URL) \
	 DBOS_APPLICATION_VERSION=$(DBOS_APPLICATION_VERSION) \
	 CONTENT_API_URL=$(CONTENT_API_URL) \
	 WORKER_HTTP_ADDR=$(WORKER_HTTP_ADDR) \
	 go run ./cmd/pipeline-worker/main.go

run-python-worker:
	@echo "Starting Python ML worker (OCR, YOLO)..."
	@echo "  DBOS URL: $(DBOS_SYSTEM_DATABASE_URL)"
	@echo "  Workflow URL: $(WORKFLOW_DATABASE_URL)"
	@echo "  Content API: $(CONTENT_API_URL)"
	@cd python-worker && \
	 DBOS_SYSTEM_DATABASE_URL=$(DBOS_SYSTEM_DATABASE_URL) \
	 WORKFLOW_DATABASE_URL=$(WORKFLOW_DATABASE_URL) \
	 DBOS_APPLICATION_VERSION=$(DBOS_APPLICATION_VERSION) \
	 CONTENT_API_URL=$(CONTENT_API_URL) \
	 WORKER_HTTP_ADDR=:$(PYTHON_WORKER_PORT) \
	 python3 main.py

dev-workers:
	@echo "Starting both workers in parallel..."
	@echo "  Go worker:     $(WORKER_HTTP_ADDR)"
	@echo "  Python worker: :$(PYTHON_WORKER_PORT)"
	@echo ""
	@echo "Press Ctrl+C to stop all workers"
	@trap 'kill 0' SIGINT; \
	 $(MAKE) run-go-worker & \
	 $(MAKE) run-python-worker & \
	 wait

dev-all:
	@echo "Starting full local development environment..."
	@$(MAKE) postgres-start || echo "PostgreSQL already running"
	@sleep 2
	@$(MAKE) migrate-workflow-up
	@echo ""
	@echo "✓ Database and migrations ready"
	@echo ""
	@$(MAKE) dev-workers
