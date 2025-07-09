# Variables
BINARY_NAME = go-fdo-server

# Build the Go project
build: tidy fmt vet
	CGO_ENABLED=0 go build -o $(BINARY_NAME)

tidy:
	go mod tidy

fmt:
	go fmt ./...

vet:
	go vet -v ./...

test:
	go test -v ./...

# Clean up the binary
clean:
	rm -f $(BINARY_NAME)

# Default target
all: build test
