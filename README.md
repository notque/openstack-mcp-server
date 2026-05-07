# openstack-mcp-server

MCP (Model Context Protocol) server for OpenStack and SAP Converged Cloud. Provides AI coding agents with typed, structured tools for querying infrastructure — 54 tools across 18 services.

## Quick Start

```bash
# 1. Install
go install github.com/notque/openstack-mcp-server/cmd/openstack-mcp-server@latest

# 2. Store your password in the system keychain (macOS)
security add-generic-password -a your-user -s openstack -w "your-password"

# 3. Add to ~/.claude/settings.json (see Configuration below)

# 4. Try asking Claude:
#    "List my servers and their status"
#    "Show quota usage for my project"
#    "What audit events happened today?"
```

## Services

### Standard OpenStack
| Service | Tools | Description |
|---------|-------|-------------|
| **Nova** (Compute) | `nova_list_servers`, `nova_get_server`, `nova_list_flavors`, `nova_server_action`* | Servers, flavors, actions |
| **Neutron** (Networking) | `neutron_list_networks`, `neutron_list_subnets`, `neutron_list_ports`, `neutron_list_security_groups` | Networks, subnets, ports, security groups |
| **Cinder** (Block Storage) | `cinder_list_volumes`, `cinder_get_volume` | Volumes |
| **Keystone** (Identity) | `keystone_list_projects`, `keystone_token_info`, `keystone_list_app_credentials`, `keystone_create_app_credential`*, `keystone_delete_app_credential`* | Projects, auth info, app credentials |
| **Designate** (DNS) | `designate_list_zones`, `designate_get_zone`, `designate_list_recordsets` | DNS zones and records |
| **Barbican** (Key Manager) | `barbican_list_secrets`, `barbican_get_secret` | Secrets metadata (no payloads) |
| **Swift** (Object Storage) | `swift_list_containers`, `swift_list_objects`, `swift_get_object_metadata` | Containers and objects |
| **Manila** (Shared Filesystems) | `manila_list_shares`, `manila_get_share` | File shares |
| **Octavia** (Load Balancer) | `octavia_list_loadbalancers`, `octavia_get_loadbalancer`, `octavia_list_listeners`, `octavia_list_pools` | Load balancers, listeners, pools |
| **Glance** (Image) | `glance_list_images`, `glance_get_image` | Images |
| **Ironic** (Bare Metal) | `ironic_list_nodes`, `ironic_get_node` | Baremetal nodes |

### SAP Converged Cloud
| Service | Tools | Description |
|---------|-------|-------------|
| **Hermes** (Audit) | `hermes_list_events`, `hermes_get_event`, `hermes_list_attributes` | CADF audit events |
| **Limes** (Quota/Usage) | `limes_get_project`, `limes_get_domain`, `limes_get_cluster` | Quota and usage reports |
| **Keppel** (Container Registry) | `keppel_list_accounts`, `keppel_list_repositories`, `keppel_list_manifests` | Container image registry |
| **Archer** (Endpoint Service) | `archer_list_services`, `archer_get_service`, `archer_list_endpoints`, `archer_get_endpoint` | Private endpoint connectivity |
| **Maia** (Prometheus) | `maia_query`, `maia_label_values`, `maia_metric_names` | PromQL queries and metrics |
| **Castellum** (Autoscaling) | `castellum_get_project_resources`, `castellum_list_pending_operations`, `castellum_list_recently_failed_operations` | Resource autoscaling |
| **Cronus** (Email) | `cronus_get_usage`, `cronus_list_templates` | Email service usage and templates |

*\* Mutating tools — disabled in read-only mode (default). Set `MCP_READ_ONLY=false` to enable.*

## Configuration

### Claude Code / Cursor

Add to your `~/.claude/settings.json`:

```json
{
  "mcpServers": {
    "openstack": {
      "command": "openstack-mcp-server",
      "env": {
        "OS_AUTH_URL": "https://identity-3.eu-de-1.cloud.sap/v3",
        "OS_USERNAME": "I-number",
        "OS_PW_CMD": "security find-generic-password -a I-number -s openstack -w",
        "OS_USER_DOMAIN_NAME": "your-domain",
        "OS_PROJECT_NAME": "your-project",
        "OS_PROJECT_DOMAIN_NAME": "your-domain",
        "OS_REGION_NAME": "eu-de-1"
      }
    }
  }
}
```

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `OS_CLOUD` | Cloud name from `clouds.yaml` | — |
| `OS_AUTH_URL` | Keystone auth URL (env-based auth) | — |
| `OS_USERNAME` | Username | — |
| `OS_PW_CMD` | Shell command to retrieve password (e.g., keychain) | — |
| `OS_PASSWORD` | Password (prefer `OS_PW_CMD` instead) | — |
| `OS_USER_DOMAIN_NAME` | User domain | — |
| `OS_PROJECT_NAME` | Project scope | — |
| `OS_PROJECT_DOMAIN_NAME` | Project domain | — |
| `OS_REGION_NAME` | Region | — |
| `OS_APPLICATION_CREDENTIAL_ID` | App credential ID (preferred for automation) | — |
| `OS_APPCRED_SECRET_CMD` | Shell command to retrieve app credential secret | — |
| `MCP_TRANSPORT` | Transport: `stdio` or `sse` | `stdio` |
| `MCP_READ_ONLY` | Set to `false` to enable mutating tools | `true` |
| `MCP_DEBUG` | Enable debug logging | `false` |
| `SAPCC_*_ENDPOINT` | Override SAP CC service endpoints | — |

## Security

### Read-Only Mode (Default)

By default, mutating tools are **disabled**:
- `nova_server_action` (start/stop/reboot servers)
- `keystone_create_application_credential`
- `keystone_delete_application_credential`

Set `MCP_READ_ONLY=false` only when you explicitly need write operations.

### Credential Isolation Architecture

```
┌─────────────┐     tool calls      ┌──────────────────┐     API calls     ┌──────────────┐
│   LLM/AI    │ ◄─────────────────► │  MCP Server      │ ◄───────────────► │  OpenStack   │
│   Client    │   (data only,       │  (holds creds    │   (authenticated  │  APIs        │
│             │    no secrets)       │   in memory)     │    with token)    │              │
└─────────────┘                     └──────────────────┘                   └──────────────┘
```

| Layer | Protection |
|-------|-----------|
| **Password** | Retrieved at startup via `OS_PW_CMD` (keychain), held only in process memory |
| **Auth Token** | Server memory only. Never included in tool responses. Auto-refreshed. |
| **Tool Responses** | All responses pass through `SanitizeResponse()` — redacts tokens, passwords, sensitive fields |
| **Ironic** | DriverInfo/Properties excluded from responses (contain BMC credentials) |

### SSE Transport Warning

SSE mode (`MCP_TRANSPORT=sse`) binds plain HTTP with no authentication. **Never expose to a network without a TLS reverse proxy.** It is intended for local development only.

## Example Prompts

Try these after setup:

- "List my servers and show which ones are in ERROR state"
- "What's the quota usage for my project?"
- "Show me the last 10 audit events for compute/server actions"
- "List all DNS zones and their recordsets"
- "Query Prometheus for the last 5 minutes of CPU usage: `rate(node_cpu_seconds_total[5m])`"
- "What load balancers exist and what pools do they have?"
- "Show me pending Castellum autoscaling operations"

## Development

```bash
# Build
make build-all

# Run linter (0 issues required)
make run-golangci-lint

# Run tests
go test ./...

# Tidy dependencies
make tidy-deps
```

## Architecture

```
cmd/openstack-mcp-server/    # CLI entry point (Cobra + go-bits)
internal/
  auth/                       # Keystone auth + service client factory
  config/                     # Configuration (env vars + optional YAML)
  server/                     # MCP server setup + tool registration
  tools/
    nova/                     # Compute (servers, flavors, actions)
    neutron/                  # Networking (networks, subnets, ports, SGs)
    cinder/                   # Block storage (volumes)
    keystone/                 # Identity (projects, app credentials)
    designate/                # DNS (zones, recordsets)
    barbican/                 # Key manager (secrets metadata)
    swift/                    # Object storage (containers, objects)
    manila/                   # Shared filesystems (shares)
    octavia/                  # Load balancer (LBs, listeners, pools)
    glance/                   # Image (images)
    ironic/                   # Bare metal (nodes)
    hermes/                   # Audit (CADF events) — SAP CC
    limes/                    # Quota/usage — SAP CC
    keppel/                   # Container registry — SAP CC
    archer/                   # Endpoint service — SAP CC
    maia/                     # Prometheus metrics — SAP CC
    castellum/                # Autoscaling — SAP CC
    cronus/                   # Email — SAP CC
    shared/                   # Helpers + response sanitization
```

## Troubleshooting

### "no cloud specified" error
Set either `OS_CLOUD` (for clouds.yaml) or `OS_AUTH_URL` (for env-based auth).

### Authentication fails
- Verify your password: `security find-generic-password -a your-user -s openstack -w`
- Check `OS_USER_DOMAIN_NAME` matches your domain (not project domain)
- Ensure `OS_AUTH_URL` ends with `/v3`

### "endpoint not found in catalog"
- Check `OS_REGION_NAME` is correct
- For SAP CC services, set the `SAPCC_*_ENDPOINT` env var as a fallback

### Tools not appearing
- Run `openstack-mcp-server --version` to verify the binary is installed
- Check Claude Code logs for startup errors
- Enable debug logging with `MCP_DEBUG=true`

### Token expired during long session
The server auto-refreshes tokens (via `AllowReauth: true`). If you still see 401 errors, restart the MCP server.

## License

Apache-2.0
