.PHONY: build test clean dev

BINARY := openstack-mcp-server
MODULE := github.com/notque/openstack-mcp-server

build:
	go build -o bin/$(BINARY) ./cmd/openstack-mcp-server

test:
	go test ./...

clean:
	rm -rf bin/

dev:
	@which air > /dev/null 2>&1 || (echo "Install air: go install github.com/air-verse/air@latest" && exit 1)
	air --build.cmd "go build -o bin/$(BINARY) ./cmd/openstack-mcp-server" --build.bin "bin/$(BINARY)"

install:
	go install ./cmd/openstack-mcp-server

tidy:
	go mod tidy

lint:
	golangci-lint run ./...
