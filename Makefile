.PHONY: postgres-start postgres-stop postgres-logs build test

# PostgreSQL configuration
POSTGRES_CONTAINER=pipeline-postgres
POSTGRES_IMAGE=postgres:17
POSTGRES_USER=pipeline
POSTGRES_PASSWORD=pwd
POSTGRES_DB=pipeline
POSTGRES_PORT=5434

# Start PostgreSQL with podman
postgres-start:
	@echo "Starting PostgreSQL container..."
	podman run -d \
		--name $(POSTGRES_CONTAINER) \
		-e POSTGRES_USER=$(POSTGRES_USER) \
		-e POSTGRES_PASSWORD=$(POSTGRES_PASSWORD) \
		-e POSTGRES_DB=$(POSTGRES_DB) \
		-p $(POSTGRES_PORT):5432 \
		$(POSTGRES_IMAGE)
	@echo "PostgreSQL started on port $(POSTGRES_PORT)"
	@echo "Connection string: postgresql://$(POSTGRES_USER):$(POSTGRES_PASSWORD)@localhost:$(POSTGRES_PORT)/$(POSTGRES_DB)"

# Stop PostgreSQL
postgres-stop:
	@echo "Stopping PostgreSQL container..."
	podman stop $(POSTGRES_CONTAINER) || true
	podman rm $(POSTGRES_CONTAINER) || true
	@echo "PostgreSQL stopped"

# View PostgreSQL logs
postgres-logs:
	podman logs -f $(POSTGRES_CONTAINER)

# Build binaries
build:
	@echo "Building pipeline-standalone..."
	go build -o pipeline-standalone ./cmd/pipeline-standalone
	@echo "Building pipeline-worker..."
	go build -o pipeline-worker ./cmd/pipeline-worker
	@echo "Build complete"

# Run tests
test:
	go test ./...

# Clean build artifacts
clean:
	rm -f pipeline-standalone pipeline-worker
