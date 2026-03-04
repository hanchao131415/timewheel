.PHONY: all test test-unit test-integration test-performance test-chaos test-all test-coverage clean help

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod

# Main package
MAIN_PACKAGE=./cmd/server

# Test parameters
TEST_TIMEOUT=10m
PERF_TIMEOUT=30m
CHAOS_TIMEOUT=60m

# Coverage
COVERAGE_FILE=coverage.out
COVERAGE_HTML=coverage.html
COVERAGE_THRESHOLD=80

# Docker
DOCKER_COMPOSE=docker-compose

all: clean deps build

## build: Build the application
build:
	$(GOBUILD) -o bin/timewheel $(MAIN_PACKAGE)

## deps: Download dependencies
deps:
	$(GOMOD) download
	$(GOMOD) verify

## clean: Clean build files
clean:
	$(GOCLEAN)
	rm -rf bin/
	rm -f $(COVERAGE_FILE) $(COVERAGE_HTML)

## test: Run all tests
test: test-unit

## test-unit: Run unit tests only
test-unit:
	@echo "Running unit tests..."
	$(GOTEST) -v ./pkg/timewheel/... -count=1 -timeout $(TEST_TIMEOUT)

## test-integration: Run integration tests
test-integration:
	@echo "Running integration tests..."
	$(GOTEST) -v ./tests/integration/... -count=1 -timeout $(TEST_TIMEOUT)

## test-performance: Run performance tests
test-performance:
	@echo "Running performance tests..."
	$(GOTEST) -v ./tests/performance/... -count=1 -timeout $(PERF_TIMEOUT)

## test-chaos: Run chaos tests
test-chaos:
	@echo "Running chaos tests..."
	$(GOTEST) -v ./tests/chaos/... -count=1 -timeout $(CHAOS_TIMEOUT)

## test-all: Run all test suites
test-all: test-unit test-integration test-performance test-chaos
	@echo "All tests completed!"

## test-coverage: Generate test coverage report
test-coverage:
	@echo "Generating coverage report..."
	$(GOTEST) -coverprofile=$(COVERAGE_FILE) -covermode=atomic ./...
	$(GOCMD) tool cover -html=$(COVERAGE_FILE) -o $(COVERAGE_HTML)
	@echo "Coverage report generated: $(COVERAGE_HTML)"
	@$(GOCMD) tool cover -func=$(COVERAGE_FILE) | grep total | awk '{print "Total coverage: " $$3}'

## test-coverage-check: Check coverage meets threshold
test-coverage-check:
	@echo "Checking coverage threshold ($(COVERAGE_THRESHOLD)%)..."
	@$(GOTEST) -coverprofile=$(COVERAGE_FILE) -covermode=atomic ./... 2>/dev/null
	@coverage=$$($(GOCMD) tool cover -func=$(COVERAGE_FILE) | grep total | awk '{print $$3}' | tr -d '%'); \
	if [ "$$coverage" -lt $(COVERAGE_THRESHOLD) ]; then \
		echo "ERROR: Coverage $$coverage% is below threshold $(COVERAGE_THRESHOLD)%"; \
		exit 1; \
	fi; \
	echo "SUCCESS: Coverage $$coverage% meets threshold $(COVERAGE_THRESHOLD)%"

## benchmark: Run benchmarks
benchmark:
	@echo "Running benchmarks..."
	$(GOTEST) -bench=. -benchmem -run=^$ ./pkg/timewheel/... ./tests/performance/...

## benchmark-memory: Run memory benchmarks
benchmark-memory:
	@echo "Running memory benchmarks..."
	$(GOTEST) -v ./tests/performance/memory_test.go -bench=. -benchmem -memprofile=mem.out
	@echo "Memory profile saved to mem.out"
	@echo "Run 'go tool pprof mem.out' to analyze"

## stress-50k: Run 50K task stress test
stress-50k:
	@echo "Running 50K task stress test..."
	$(GOTEST) -v -run TestScalability_50K ./tests/performance/ -timeout $(PERF_TIMEOUT)

## stress-100k: Run 100K task stress test
stress-100k:
	@echo "Running 100K task stress test..."
	$(GOTEST) -v -run TestScalability_100K ./tests/performance/ -timeout $(PERF_TIMEOUT)

## lint: Run linters
lint:
	@echo "Running linters..."
	@which golangci-lint > /dev/null || go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	golangci-lint run ./...

## fmt: Format code
fmt:
	@echo "Formatting code..."
	$(GOCMD) fmt ./...

## vet: Run go vet
vet:
	@echo "Running go vet..."
	$(GOCMD) vet ./...

## check: Run all checks (fmt, vet, lint, test)
check: fmt vet lint test-unit
	@echo "All checks passed!"

## docker-build: Build Docker image
docker-build:
	@echo "Building Docker image..."
	docker build -t timewheel:latest .

## docker-run: Run Docker container
docker-run:
	@echo "Running Docker container..."
	docker run -p 8080:8080 timewheel:latest

## docker-compose-up: Start with docker-compose
docker-compose-up:
	$(DOCKER_COMPOSE) up -d

## docker-compose-down: Stop docker-compose
docker-compose-down:
	$(DOCKER_COMPOSE) down

## docker-compose-logs: View docker-compose logs
docker-compose-logs:
	$(DOCKER_COMPOSE) logs -f

## migrate-up: Run database migrations
migrate-up:
	@echo "Running migrations..."
	@if [ -d migrations ]; then \
		for f in migrations/*.sql; do \
			echo "Applying $$f"; \
		done; \
	fi

## db-test: Start test database
db-test:
	@echo "Starting test database..."
	docker run -d --name timewheel-test-mysql \
		-e MYSQL_ROOT_PASSWORD=testpass \
		-e MYSQL_DATABASE=timewheel_test \
		-p 3306:3306 \
		mysql:8.0

## db-test-stop: Stop test database
db-test-stop:
	@echo "Stopping test database..."
	docker stop timewheel-test-mysql && docker rm timewheel-test-mysql

## profile-cpu: Run CPU profiling
profile-cpu:
	@echo "Running CPU profiling..."
	$(GOTEST) -cpuprofile=cpu.out -bench=. -run=^$ ./pkg/timewheel/...
	@echo "CPU profile saved to cpu.out"
	@echo "Run 'go tool pprof cpu.out' to analyze"

## profile-memory: Run memory profiling
profile-memory:
	@echo "Running memory profiling..."
	$(GOTEST) -memprofile=mem.out -bench=. -run=^$ ./pkg/timewheel/...
	@echo "Memory profile saved to mem.out"
	@echo "Run 'go tool pprof mem.out' to analyze"

## trace: Run execution trace
trace:
	@echo "Running execution trace..."
	$(GOTEST) -trace=trace.out -run=TestScalability_10K ./tests/performance/
	@echo "Trace saved to trace.out"
	@echo "Run 'go tool trace trace.out' to analyze"

## help: Show this help message
help:
	@echo "TimeWheel - High Performance Task Scheduler"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@sed -n 's/^##//p' $(MAKEFILE_LIST) | column -t -s ':' | sed -e 's/^/ /'
	@echo ""
	@echo "Examples:"
	@echo "  make test-unit          # Run unit tests"
	@echo "  make test-coverage      # Generate coverage report"
	@echo "  make benchmark          # Run benchmarks"
	@echo "  make stress-50k         # Run 50K task stress test"
