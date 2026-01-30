.PHONY: build test run clean lint fmt vet

BINARY_NAME=matcha
BUILD_DIR=bin

generate_gif:
	alias matcha="go run ."
	vhs demo.tape
	mv demo.gif public/assets/demo.gif

build:
	go build -o $(BUILD_DIR)/$(BINARY_NAME) .

run:
	go run .

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
