<!--
SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company
SPDX-License-Identifier: Apache-2.0
-->

# Contributing to openstack-mcp-server

Thank you for your interest in contributing to the OpenStack MCP Server!

## How to Contribute

1. Fork the repository
2. Create a feature branch (`git checkout -b feat/my-feature`)
3. Make your changes
4. Ensure `make check` passes (build + lint + tests)
5. Commit with [conventional commits](https://www.conventionalcommits.org/)
6. Push to your fork and open a Pull Request

## Development Setup

```bash
# Install dependencies
go mod download

# Build
make build-all

# Run linter
make run-golangci-lint

# Run tests
go test ./...
```

## Code Style

- All `.go` files must have SPDX license headers
- Use `go-bits/logg` for logging (not slog, zap, or logrus)
- Use `go-bits/osext` for environment variable handling
- Use `go-bits/must` for fatal startup errors
- Follow import grouping: stdlib / external / local
- All tool handlers must use `shared.ToolResult()`/`shared.ToolError()` for responses

## Adding a New Service Module

1. Create `internal/tools/<service>/<service>.go`
2. Implement `Register(s *mcpserver.MCPServer, provider *auth.Provider)`
3. Add client method to `internal/auth/provider.go` if needed
4. Register in `internal/server/server.go`
5. Update the test in `internal/server/server_test.go`

## Security

- Never expose auth tokens in tool responses
- Use field allowlists (not blocklists) when marshaling structs
- Validate user inputs that become URL path components

## License

By contributing, you agree that your contributions will be licensed under Apache-2.0.
