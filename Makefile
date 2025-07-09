# Build the Go project
build: tidy fmt vet
	CGO_ENABLED=0 go build

tidy:
	go mod tidy

fmt:
	go fmt ./...

vet:
	go vet -v ./...

test:
	go test -v ./...

# Default target
all: build test
