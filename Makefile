.PHONY: build test run run-log clean lint fmt vet build-full generate_screenshots

BINARY_NAME=matcha
BUILD_DIR=bin

generate_gif:
	alias matcha="go run ."
	vhs demo.tape
	mv demo.gif public/assets/demo.gif

generate_screenshots:
	@mkdir -p docs/docs/assets/features/
	@for tape in screenshots/*.tape; do \
		[ "$$(basename $$tape)" = "common.tape" ] && continue; \
		name=$$(basename "$$tape" .tape); \
		echo "==> Generating screenshot: $$name"; \
		vhs "$$tape" || echo "Warning: $$name failed"; \
	done
	@mv screenshots/*.png docs/docs/assets/features/ 2>/dev/null || true
	@rm -f screenshots/*.gif 2>/dev/null || true
	@echo "Screenshots saved to docs/docs/assets/features/"

build:
	go build -o $(BUILD_DIR)/$(BINARY_NAME) .

build-full:
	@echo "Building with version information..."
	@VERSION=$$(git describe --tags --abbrev=0 2>/dev/null || echo "dev"); \
	COMMIT=$$(git rev-parse --short HEAD 2>/dev/null || echo "unknown"); \
	DATE=$$(date +%Y-%m-%d); \
	echo "Version: $$VERSION"; \
	echo "Commit: $$COMMIT"; \
	echo "Date: $$DATE"; \
	go build -ldflags="-X 'main.version=$$VERSION' -X 'main.commit=$$COMMIT' -X 'main.date=$$DATE'" -o $(BUILD_DIR)/$(BINARY_NAME)-full .;

run:
	go run .

run-log:
	go run . --debug --logs

test:
	go test ./...

test-verbose:
	go test -v ./...

test-coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

clean:
	rm -rf $(BUILD_DIR)
	rm -f coverage.out coverage.html

fmt:
	go fmt ./...

vet:
	go vet ./...

lint: fmt vet

all: lint test build
