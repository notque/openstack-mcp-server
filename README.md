<!--
SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company
SPDX-License-Identifier: Apache-2.0
-->

# openstack-mcp-server

MCP (Model Context Protocol) server for OpenStack and SAP Converged Cloud. Provides AI coding agents with typed, structured tools for managing infrastructure — 86 tools across 18 services (72 read + 14 write).

## Quick Start

```bash
# 1. Install
go install github.com/notque/openstack-mcp-server/cmd/openstack-mcp-server@latest

# 2. Store your password in the system keychain (macOS)
security add-generic-password -a your-user -s openstack -w "your-password"

# 3. Register as an MCP server (see Configuration below)
claude mcp add openstack openstack-mcp-server \
  -e OS_AUTH_URL=https://identity-3.eu-de-1.cloud.sap/v3 \
  -e OS_USERNAME=your-user \
  -e OS_PW_CMD="security find-generic-password -a your-user -s openstack -w" \
  -e OS_USER_DOMAIN_NAME=your-domain \
  -e OS_PROJECT_NAME=your-project \
  -e OS_PROJECT_DOMAIN_NAME=your-domain \
  -e OS_REGION_NAME=eu-de-1

# 4. Try asking Claude:
#    "List my servers and their status"
#    "Show quota usage for my project"
#    "What audit events happened today?"
```

## Services

### Standard OpenStack
| Service | Tools | Description |
|---------|-------|-------------|
| **Nova** (Compute) | `nova_list_servers`, `nova_get_server`, `nova_list_flavors`, `nova_list_keypairs`, `nova_list_availability_zones`, `nova_get_quotas`, `nova_server_action`*, `nova_create_server`* | Servers, flavors, keypairs, AZs, quotas, actions |
| **Neutron** (Networking) | `neutron_list_networks`, `neutron_list_subnets`, `neutron_list_ports`, `neutron_list_security_groups`, `neutron_list_routers`, `neutron_list_floating_ips`, `neutron_create_security_group_rule`*, `neutron_delete_security_group_rule`* | Networks, subnets, ports, security groups, routers, floating IPs |
| **Cinder** (Block Storage) | `cinder_list_volumes`, `cinder_get_volume`, `cinder_list_snapshots`, `cinder_get_snapshot`, `cinder_list_volume_types`, `cinder_get_quotas`, `cinder_create_volume`*, `cinder_delete_volume`* | Volumes, snapshots, types, quotas |
| **Keystone** (Identity) | `keystone_list_projects`, `keystone_token_info`, `keystone_list_application_credentials`, `keystone_list_domains`, `keystone_list_users`, `keystone_list_roles`, `keystone_create_application_credential`*, `keystone_delete_application_credential`* | Projects, auth info, domains, users, roles, app credentials |
| **Designate** (DNS) | `designate_list_zones`, `designate_get_zone`, `designate_list_recordsets`, `designate_create_recordset`*, `designate_delete_recordset`* | DNS zones and records |
| **Barbican** (Key Manager) | `barbican_list_secrets`, `barbican_get_secret` | Secrets metadata (no payloads) |
| **Swift** (Object Storage) | `swift_list_containers`, `swift_list_objects`, `swift_get_object_metadata`, `swift_upload_object`*, `swift_delete_object`* | Containers and objects |
| **Manila** (Shared Filesystems) | `manila_list_shares`, `manila_get_share`, `manila_list_access_rules`, `manila_list_share_networks` | File shares, access rules, share networks |
| **Octavia** (Load Balancer) | `octavia_list_loadbalancers`, `octavia_get_loadbalancer`, `octavia_list_listeners`, `octavia_list_pools`, `octavia_list_members`, `octavia_list_healthmonitors`, `octavia_list_l7policies`, `octavia_create_loadbalancer`*, `octavia_delete_loadbalancer`* | Load balancers, listeners, pools, members, health monitors, L7 policies |
| **Glance** (Image) | `glance_list_images`, `glance_get_image` | Images |
| **Ironic** (Bare Metal) | `ironic_list_nodes`, `ironic_get_node`, `ironic_list_node_ports`, `ironic_list_allocations` | Baremetal nodes, ports, allocations |

### SAP Converged Cloud
| Service | Tools | Description |
|---------|-------|-------------|
| **Hermes** (Audit) | `hermes_list_events`, `hermes_get_event`, `hermes_list_attributes` | CADF audit events |
| **Limes** (Quota/Usage) | `limes_get_project_quota`, `limes_get_domain_quota`, `limes_get_cluster_quota` | Quota and usage reports |
| **Keppel** (Container Registry) | `keppel_list_accounts`, `keppel_list_repositories`, `keppel_list_manifests`, `keppel_get_vulnerability_report` | Container image registry, vulnerability scanning |
| **Archer** (Endpoint Service) | `archer_list_services`, `archer_get_service`, `archer_list_endpoints`, `archer_get_endpoint` | Private endpoint connectivity |
| **Maia** (Prometheus) | `maia_query`, `maia_query_range`, `maia_label_values`, `maia_metric_names` | PromQL instant and range queries, metrics |
| **Castellum** (Autoscaling) | `castellum_get_project_resources`, `castellum_list_pending_operations`, `castellum_list_recently_failed_operations` | Resource autoscaling |
| **Cronus** (Email) | `cronus_get_usage`, `cronus_list_templates` | Email service usage and templates |

*\* Mutating tools (14 total) — disabled in read-only mode (default). Set `MCP_READ_ONLY=false` to enable.*

## Configuration

### Claude Code

Register the MCP server using the CLI:

```bash
claude mcp add openstack openstack-mcp-server \
  -e OS_AUTH_URL=https://identity-3.eu-de-1.cloud.sap/v3 \
  -e OS_USERNAME=your-user \
  -e OS_PW_CMD="security find-generic-password -a your-user -s openstack -w" \
  -e OS_USER_DOMAIN_NAME=your-domain \
  -e OS_PROJECT_NAME=your-project \
  -e OS_PROJECT_DOMAIN_NAME=your-domain \
  -e OS_REGION_NAME=eu-de-1
```

This writes to `~/.claude.json` which Claude Code reads at session start. Use `--scope project` to scope to a single repo (writes to `.mcp.json`).

To verify: `claude mcp list`

### Cursor / Other MCP Clients

Add to your MCP client's configuration file (e.g., `.cursor/mcp.json`):

```json
{
  "mcpServers": {
    "openstack": {
      "command": "openstack-mcp-server",
      "env": {
        "OS_AUTH_URL": "https://identity-3.eu-de-1.cloud.sap/v3",
        "OS_USERNAME": "your-user",
        "OS_PW_CMD": "security find-generic-password -a your-user -s openstack -w",
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

### Four-Layer Safety Architecture

| Layer | Mechanism | Effect |
|-------|-----------|--------|
| **1. Read-Only Mode** | `MCP_READ_ONLY=true` (default) | Mutating tools are not registered — invisible to the LLM |
| **2. Confirmed Pattern** | Two-call execution | First call returns a preview of what will happen; second call with `confirmed=true` executes |
| **3. Semantic Guardrails** | Domain-specific validation | Rejects dangerous operations outright (e.g., 0.0.0.0/0 on SSH, deleting in-use volumes) |
| **4. Credential Isolation** | Secrets held in server memory only | Auth tokens and passwords never reach the LLM |

### Write Safety: The Confirmed Pattern

All 14 write tools implement a two-call safety pattern:

```
1st call (confirmed absent/false):
   → Returns PREVIEW: "Will DELETE volume 'db-backup' (abc123), 50GiB, status: available"

2nd call (confirmed=true):
   → Executes the operation
```

This gives the AI agent (and the human supervising it) a chance to review what will happen before any state changes.

### Semantic Guardrails

Write tools include domain-specific safety rules that reject dangerous operations before they reach the confirmation step:

| Service | Rule | Rationale |
|---------|------|-----------|
| **Neutron** | Rejects ingress 0.0.0.0/0 on ports 22, 3389, 3306, 5432 | Prevents accidental world-open SSH/RDP/DB access |
| **Cinder** | Rejects delete on status `in-use` | Prevents data loss from deleting attached volumes |
| **Designate** | Enforces CNAME singleton per name | DNS RFC compliance (CNAME can't coexist with other records) |
| **Swift** | `safe_write` option uses `If-None-Match:*` | Prevents accidental overwrites of existing objects |
| **Octavia** | Cascade delete requires explicit opt-in | Prevents accidental deletion of listeners, pools, and members |

### Read-Only Mode (Default)

By default, all 14 mutating tools are **disabled** (`MCP_READ_ONLY=true`). Set `MCP_READ_ONLY=false` only when you explicitly need write operations. The write tools are:

- `nova_server_action`, `nova_create_server`
- `neutron_create_security_group_rule`, `neutron_delete_security_group_rule`
- `cinder_create_volume`, `cinder_delete_volume`
- `designate_create_recordset`, `designate_delete_recordset`
- `swift_upload_object`, `swift_delete_object`
- `octavia_create_loadbalancer`, `octavia_delete_loadbalancer`
- `keystone_create_application_credential`, `keystone_delete_application_credential`

### Tool Annotations (Human-in-the-Loop)

All tools declare their intent via [MCP tool annotations](https://modelcontextprotocol.io/specification/2025-03-26/server/tools#annotations):

- **Read-only tools** (72 tools): Annotated with `readOnlyHint: true`. Clients may auto-approve these.
- **Destructive tools** (14 tools): Annotated with `destructiveHint: true`. Clients **must prompt the user** before execution.

This means even when `MCP_READ_ONLY=false` enables destructive tools, the MCP client (Claude Code, Cursor, etc.) will still ask "Allow this action?" before executing. The server declares intent, the client enforces the gate.

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

With write tools enabled (`MCP_READ_ONLY=false`):
- "Create a 100GiB SSD volume named 'db-data' in AZ eu-de-1a"
- "Add an A record for api.example.com pointing to 10.0.1.50"
- "Create a security group rule allowing TCP 443 from 10.0.0.0/8"
- "Create a load balancer on subnet abc123 named 'web-lb'"

## Companion: Agent Toolkit

For enhanced AI workflows, pair this MCP server with the [OpenStack Agent Toolkit](https://github.com/notque/openstack-agent-toolkit) — a Claude Code plugin providing domain knowledge, operational skills, and safety hooks for SAP Converged Cloud infrastructure.

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
