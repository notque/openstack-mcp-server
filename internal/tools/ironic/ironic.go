// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

// Package ironic provides MCP tools for OpenStack Bare Metal (Ironic) operations.
package ironic

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/v1/allocations"
	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/v1/nodes"
	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/v1/ports"
	"github.com/gophercloud/gophercloud/v2/pagination"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/notque/openstack-mcp-server/internal/auth"
	"github.com/notque/openstack-mcp-server/internal/tools/shared"
)

// Register adds all Ironic tools to the MCP server.
// When readOnly is true, mutating tools are not registered.
// When admin is true, admin-only tools (chassis, power state) are registered.
func Register(s *mcpserver.MCPServer, provider *auth.Provider, readOnly bool, admin bool) {
	s.AddTool(listNodesTool, listNodesHandler(provider))
	s.AddTool(getNodeTool, getNodeHandler(provider))
	s.AddTool(listNodePortsTool, listNodePortsHandler(provider))
	s.AddTool(listAllocationsTool, listAllocationsHandler(provider))
	s.AddTool(listPortgroupsTool, listPortgroupsHandler(provider))
	if admin {
		s.AddTool(listChassisTool, listChassisHandler(provider))
		if !readOnly {
			s.AddTool(nodeChangePowerStateTool, nodeChangePowerStateHandler(provider))
		}
	}
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

var listNodePortsTool = mcp.NewTool("ironic_list_node_ports",
	mcp.WithDescription("List network ports (NICs) for a baremetal node. Returns port UUID, address (MAC), node UUID, PXE enabled, and physical network."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithString("node_id", mcp.Required(), mcp.Description("The UUID of the baremetal node")),
)

func listNodePortsHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.BareMetalClient()
		if err != nil {
			return shared.ToolError("failed to get baremetal client: %v", err), nil
		}

		nodeID := shared.StringParam(request, "node_id")
		if nodeID == "" {
			return shared.ToolError("node_id is required"), nil
		}
		if errResult := shared.ValidateUUID(nodeID, "node_id"); errResult != nil {
			return errResult, nil
		}

		opts := ports.ListOpts{
			NodeUUID: nodeID,
		}

		var result []map[string]any
		err = ports.List(client, opts).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
			portList, err := ports.ExtractPorts(page)
			if err != nil {
				return false, err
			}
			for _, p := range portList {
				result = append(result, map[string]any{
					"uuid":             p.UUID,
					"address":          p.Address,
					"node_uuid":        p.NodeUUID,
					"pxe_enabled":      p.PXEEnabled,
					"physical_network": p.PhysicalNetwork,
				})
			}
			return true, nil
		})
		if err != nil {
			return shared.ToolError("failed to list ports for node %s: %v", nodeID, err), nil
		}

		out, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

// --- Allocations ---

var listAllocationsTool = mcp.NewTool("ironic_list_allocations",
	mcp.WithDescription("List baremetal node allocations. Returns allocation UUID, node UUID, state, resource class, and name."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithString("node_id", mcp.Description("Filter by node UUID")),
	mcp.WithString("resource_class", mcp.Description("Filter by resource class")),
)

func listAllocationsHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.BareMetalClient()
		if err != nil {
			return shared.ToolError("failed to get baremetal client: %v", err), nil
		}

		opts := allocations.ListOpts{}
		if v := shared.StringParam(request, "node_id"); v != "" {
			if errResult := shared.ValidateUUID(v, "node_id"); errResult != nil {
				return errResult, nil
			}
			opts.Node = v
		}
		if v := shared.StringParam(request, "resource_class"); v != "" {
			opts.ResourceClass = v
		}

		result := make([]map[string]any, 0)
		err = allocations.List(client, opts).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
			allocationList, err := allocations.ExtractAllocations(page)
			if err != nil {
				return false, err
			}
			for _, a := range allocationList {
				result = append(result, map[string]any{
					"uuid":           a.UUID,
					"node_uuid":      a.NodeUUID,
					"state":          a.State,
					"resource_class": a.ResourceClass,
					"name":           a.Name,
				})
			}
			return true, nil
		})
		if err != nil {
			return shared.ToolError("failed to list allocations: %v", err), nil
		}

		out, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

// --- Portgroups ---

var listPortgroupsTool = mcp.NewTool("ironic_list_portgroups",
	mcp.WithDescription("List port groups for baremetal nodes. Returns UUID, name, MAC address, node UUID, and bonding mode."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithString("node_id", mcp.Description("Filter by node UUID")),
)

func listPortgroupsHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.BareMetalClient()
		if err != nil {
			return shared.ToolError("failed to get baremetal client: %v", err), nil
		}

		url := client.ServiceURL("portgroups")
		if v := shared.StringParam(request, "node_id"); v != "" {
			if errResult := shared.ValidateUUID(v, "node_id"); errResult != nil {
				return errResult, nil
			}
			url = client.ServiceURL("portgroups") + "?node=" + v
		}

		var response struct {
			Portgroups []struct {
				UUID     string `json:"uuid"`
				Name     string `json:"name"`
				Address  string `json:"address"`
				NodeUUID string `json:"node_uuid"`
				Mode     string `json:"mode"`
			} `json:"portgroups"`
		}

		_, err = client.Get(ctx, url, &response, &gophercloud.RequestOpts{
			OkCodes: []int{200},
		})
		if err != nil {
			return shared.ToolError("failed to list portgroups: %v", err), nil
		}

		result := make([]map[string]any, 0, len(response.Portgroups))
		for _, pg := range response.Portgroups {
			result = append(result, map[string]any{
				"uuid":      pg.UUID,
				"name":      pg.Name,
				"address":   pg.Address,
				"node_uuid": pg.NodeUUID,
				"mode":      pg.Mode,
			})
		}

		out, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

// --- Chassis (Admin) ---

var listChassisTool = mcp.NewTool("ironic_list_chassis",
	mcp.WithDescription("[Admin] List baremetal chassis. Requires admin role."),
	mcp.WithReadOnlyHintAnnotation(true),
)

func listChassisHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.BareMetalClient()
		if err != nil {
			return shared.ToolError("failed to get baremetal client: %v", err), nil
		}

		var response struct {
			Chassis []struct {
				UUID        string         `json:"uuid"`
				Description string         `json:"description"`
				Extra       map[string]any `json:"extra"`
			} `json:"chassis"`
		}

		_, err = client.Get(ctx, client.ServiceURL("chassis"), &response, &gophercloud.RequestOpts{
			OkCodes: []int{200},
		})
		if err != nil {
			return shared.ToolError("failed to list chassis: %v", err), nil
		}

		result := make([]map[string]any, 0, len(response.Chassis))
		for _, c := range response.Chassis {
			result = append(result, map[string]any{
				"uuid":        c.UUID,
				"description": c.Description,
				"extra":       c.Extra,
			})
		}

		out, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

// --- Power State (Admin Write) ---

var nodeChangePowerStateTool = mcp.NewTool("ironic_node_power_state",
	mcp.WithDescription("[Admin] Change the power state of a baremetal node. Requires admin role."),
	mcp.WithDestructiveHintAnnotation(true),
	mcp.WithString("node_id", mcp.Required(), mcp.Description("The UUID or name of the baremetal node")),
	mcp.WithString("target", mcp.Required(), mcp.Description("Target power state: 'power on', 'power off', or 'rebooting'")),
	mcp.WithBoolean("confirmed", mcp.Description("Set to true to execute. Without this, returns a preview of the action.")),
)

func nodeChangePowerStateHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
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

		target := shared.StringParam(request, "target")
		if target == "" {
			return shared.ToolError("target is required"), nil
		}

		// Validate the target power state.
		var powerTarget nodes.TargetPowerState
		switch target {
		case "power on":
			powerTarget = nodes.PowerOn
		case "power off":
			powerTarget = nodes.PowerOff
		case "rebooting":
			powerTarget = nodes.Rebooting
		default:
			return shared.ToolError("target must be 'power on', 'power off', or 'rebooting' (got: %q)", target), nil
		}

		preview := fmt.Sprintf("Will change power state of node %s to %q", nodeID, target)
		if result := shared.RequireConfirmation(request, preview); result != nil {
			return result, nil
		}

		opts := nodes.PowerStateOpts{
			Target: powerTarget,
		}

		err = nodes.ChangePowerState(ctx, client, nodeID, opts).ExtractErr()
		if err != nil {
			return shared.ToolError("failed to change power state of node %s: %v", nodeID, err), nil
		}

		return shared.ToolResult(fmt.Sprintf("Successfully initiated power state change of node %s to %q.", nodeID, target)), nil
	}
}
