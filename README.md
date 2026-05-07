# openstack-mcp-server

MCP (Model Context Protocol) server for OpenStack and SAP Converged Cloud. Provides AI coding agents with typed, structured tools for interacting with OpenStack services.

## Features

### Standard OpenStack Services
- **Nova** (Compute): List/get servers, flavors, server actions (start/stop/reboot)
- **Neutron** (Networking): List networks, subnets, ports, security groups
- **Cinder** (Block Storage): List/get volumes
- **Keystone** (Identity): List projects, token info

### SAP Converged Cloud Services
- **Hermes** (Audit): List/get audit events in CADF format, list attribute values
- **Limes** (Quota/Usage): Get project/domain/cluster quota and usage reports
- **Keppel** (Container Registry): List accounts, repositories, manifests
- **Archer** (Endpoint Service): List/get services and endpoints for private connectivity
- **Maia** (Prometheus-as-a-Service): PromQL queries, label values, metric names

## Prerequisites

- Go 1.22+
- OpenStack `clouds.yaml` configured (see [Configuration](#configuration))
- Access to an SAP Converged Cloud region (for SAP CC-specific tools)

## Installation

```bash
go install github.com/notque/openstack-mcp-server/cmd/openstack-mcp-server@latest
```

Or build from source:

```bash
make build
```

## Configuration

### clouds.yaml

The server uses standard OpenStack `clouds.yaml` for authentication:

```yaml
# ~/.config/openstack/clouds.yaml
clouds:
  sapcc-eu-de-1:
    auth:
      auth_url: https://identity-3.eu-de-1.cloud.sap/v3
      project_name: my-project
      project_domain_name: my-domain
      user_domain_name: my-domain
      username: my-user
      password: my-password  # or use application credentials
    region_name: eu-de-1
    interface: public
```

### Environment Variables

| Variable | Description |
|----------|-------------|
| `OS_CLOUD` | Cloud name from clouds.yaml (required) |
| `OS_REGION_NAME` | Region override |
| `MCP_TRANSPORT` | Transport: `stdio` (default) or `sse` |
| `SAPCC_KEPPEL_ENDPOINT` | Override Keppel endpoint URL |
| `SAPCC_ARCHER_ENDPOINT` | Override Archer endpoint URL |
| `SAPCC_HERMES_ENDPOINT` | Override Hermes endpoint URL |
| `SAPCC_MAIA_ENDPOINT` | Override Maia endpoint URL |
| `SAPCC_LIMES_ENDPOINT` | Override Limes endpoint URL |

### Claude Code Integration

Add to your `.mcp.json`:

```json
{
  "mcpServers": {
    "openstack": {
      "command": "openstack-mcp-server",
      "env": {
        "OS_CLOUD": "sapcc-eu-de-1"
      }
    }
  }
}
```

## Transport Modes

### stdio (default)
For local use with Claude Code, Cursor, etc. The MCP server communicates via stdin/stdout.

### SSE
For remote/shared use. Starts an HTTP server with Server-Sent Events transport:

```bash
MCP_TRANSPORT=sse openstack-mcp-server
# Listens on :8080 by default
```

## Development

```bash
# Build
make build

# Run tests
make test

# Run with hot-reload (requires air)
make dev
```

## Architecture

```
cmd/openstack-mcp-server/    # Entry point
internal/
  auth/                       # Keystone auth + service client factory
  config/                     # Configuration loading (clouds.yaml, env vars)
  server/                     # MCP server setup + tool registration
  tools/
    nova/                     # Compute tools
    neutron/                  # Networking tools
    cinder/                   # Block storage tools
    keystone/                 # Identity tools
    hermes/                   # Audit tools (SAP CC)
    limes/                    # Quota/usage tools (SAP CC)
    keppel/                   # Container registry tools (SAP CC)
    archer/                   # Endpoint service tools (SAP CC)
    maia/                     # Prometheus metrics tools (SAP CC)
```

## License

Apache-2.0
