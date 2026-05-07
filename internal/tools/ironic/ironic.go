// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

// Package ironic provides MCP tools for OpenStack Bare Metal (Ironic) operations.
package ironic

import (
	"context"
	"encoding/json"

	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/v1/nodes"
	"github.com/gophercloud/gophercloud/v2/pagination"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/notque/openstack-mcp-server/internal/auth"
	"github.com/notque/openstack-mcp-server/internal/tools/shared"
)

// Register adds all Ironic tools to the MCP server.
func Register(s *mcpserver.MCPServer, provider *auth.Provider) {
	s.AddTool(listNodesTool, listNodesHandler(provider))
	s.AddTool(getNodeTool, getNodeHandler(provider))
}

var listNodesTool = mcp.NewTool("ironic_list_nodes",
	mcp.WithDescription("List baremetal nodes in Ironic. Returns UUID, name, provision state, power state, maintenance status, driver, resource class, and instance UUID."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithString("provision_state", mcp.Description("Filter by provision state (e.g., 'active', 'available', 'deploying', 'error')")),
	mcp.WithString("maintenance", mcp.Description("Filter by maintenance mode ('true' to show only nodes in maintenance)")),
	mcp.WithString("driver", mcp.Description("Filter by driver name (e.g., 'ipmi', 'redfish')")),
	mcp.WithString("resource_class", mcp.Description("Filter by resource class (e.g., 'baremetal')")),
	mcp.WithString("instance_uuid", mcp.Description("Filter by the UUID of the Nova instance running on the node")),
	mcp.WithString("fault", mcp.Description("Filter by fault type (e.g., 'power failure', 'clean failure')")),
	mcp.WithString("owner", mcp.Description("Filter by node owner project UUID")),
)

var getNodeTool = mcp.NewTool("ironic_get_node",
	mcp.WithDescription("Get detailed information about a specific baremetal node."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithString("node_id", mcp.Required(), mcp.Description("The UUID or name of the baremetal node")),
)

func listNodesHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.BareMetalClient()
		if err != nil {
			return shared.ToolError("failed to get baremetal client: %v", err), nil
		}

		opts := nodes.ListOpts{
			Driver:        shared.StringParam(request, "driver"),
			ResourceClass: shared.StringParam(request, "resource_class"),
			InstanceUUID:  shared.StringParam(request, "instance_uuid"),
			Fault:         shared.StringParam(request, "fault"),
			Owner:         shared.StringParam(request, "owner"),
		}

		if provState := shared.StringParam(request, "provision_state"); provState != "" {
			opts.ProvisionState = nodes.ProvisionState(provState)
		}

		if shared.StringParam(request, "maintenance") == "true" {
			opts.Maintenance = true
		}

		var result []map[string]any
		err = nodes.ListDetail(client, opts).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
			nodeList, err := nodes.ExtractNodes(page)
			if err != nil {
				return false, err
			}
			for _, n := range nodeList {
				result = append(result, map[string]any{
					"uuid":            n.UUID,
					"name":            n.Name,
					"provision_state": n.ProvisionState,
					"power_state":     n.PowerState,
					"maintenance":     n.Maintenance,
					"driver":          n.Driver,
					"resource_class":  n.ResourceClass,
					"instance_uuid":   n.InstanceUUID,
				})
			}
			return true, nil
		})
		if err != nil {
			return shared.ToolError("failed to list baremetal nodes: %v", err), nil
		}

		out, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

func getNodeHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.BareMetalClient()
		if err != nil {
			return shared.ToolError("failed to get baremetal client: %v", err), nil
		}

		nodeID := shared.StringParam(request, "node_id")
		if nodeID == "" {
			return shared.ToolError("node_id is required"), nil
		}
		if errResult := shared.ValidatePathSegment(nodeID, "node_id"); errResult != nil {
			return errResult, nil
		}

		node, err := nodes.Get(ctx, client, nodeID).Extract()
		if err != nil {
			return shared.ToolError("failed to get baremetal node %s: %v", nodeID, err), nil
		}

		// SECURITY: Use allowlist of safe fields. DriverInfo, DriverInternalInfo,
		// InstanceInfo, and Properties are intentionally omitted as they may contain
		// BMC credentials (IPMI passwords, Redfish credentials, iDRAC secrets) or
		// arbitrary operator-managed key/value data.
		safe := map[string]any{
			"uuid":               node.UUID,
			"name":               node.Name,
			"provision_state":    node.ProvisionState,
			"power_state":        node.PowerState,
			"target_power_state": node.TargetPowerState,
			"maintenance":        node.Maintenance,
			"maintenance_reason": node.MaintenanceReason,
			"fault":              node.Fault,
			"last_error":         node.LastError,
			"driver":             node.Driver,
			"resource_class":     node.ResourceClass,
			"instance_uuid":      node.InstanceUUID,
			"conductor_group":    node.ConductorGroup,
			"conductor":          node.Conductor,
			"owner":              node.Owner,
			"lessee":             node.Lessee,
			"description":        node.Description,
			"created_at":         node.CreatedAt,
			"updated_at":         node.UpdatedAt,
		}

		out, err := json.MarshalIndent(safe, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}
