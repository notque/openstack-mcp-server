// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

// Package neutron provides MCP tools for OpenStack Networking (Neutron) operations.
package neutron

import (
	"context"
	"encoding/json"

	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/extensions/security/groups"
	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/networks"
	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/ports"
	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/subnets"
	"github.com/gophercloud/gophercloud/v2/pagination"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/notque/openstack-mcp-server/internal/auth"
	"github.com/notque/openstack-mcp-server/internal/tools/shared"
)

// Register adds all Neutron tools to the MCP server.
func Register(s *mcpserver.MCPServer, provider *auth.Provider) {
	s.AddTool(listNetworksTool, listNetworksHandler(provider))
	s.AddTool(listSubnetsTool, listSubnetsHandler(provider))
	s.AddTool(listPortsTool, listPortsHandler(provider))
	s.AddTool(listSecGroupsTool, listSecGroupsHandler(provider))
}

var listNetworksTool = mcp.NewTool("neutron_list_networks",
	mcp.WithDescription("List networks in the current project. Returns network ID, name, status, subnets, and admin state."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithString("name", mcp.Description("Filter by network name")),
	mcp.WithString("status", mcp.Description("Filter by network status (ACTIVE, DOWN, BUILD, ERROR)")),
)

var listSubnetsTool = mcp.NewTool("neutron_list_subnets",
	mcp.WithDescription("List subnets in the current project. Returns subnet ID, name, CIDR, gateway, and network ID."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithString("network_id", mcp.Description("Filter by network ID")),
)

var listPortsTool = mcp.NewTool("neutron_list_ports",
	mcp.WithDescription("List ports in the current project. Returns port ID, name, status, MAC, fixed IPs, and device owner."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithString("network_id", mcp.Description("Filter by network ID")),
	mcp.WithString("device_id", mcp.Description("Filter by device (server) ID")),
	mcp.WithString("status", mcp.Description("Filter by port status (ACTIVE, DOWN, BUILD)")),
)

var listSecGroupsTool = mcp.NewTool("neutron_list_security_groups",
	mcp.WithDescription("List security groups in the current project with their rules."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithString("name", mcp.Description("Filter by security group name")),
)

func listNetworksHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.NetworkClient()
		if err != nil {
			return shared.ToolError("failed to get network client: %v", err), nil
		}

		opts := networks.ListOpts{
			Name:   shared.StringParam(request, "name"),
			Status: shared.StringParam(request, "status"),
		}

		var result []map[string]any
		err = networks.List(client, opts).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
			nets, err := networks.ExtractNetworks(page)
			if err != nil {
				return false, err
			}
			for _, n := range nets {
				result = append(result, map[string]any{
					"id":          n.ID,
					"name":        n.Name,
					"status":      n.Status,
					"subnets":     n.Subnets,
					"admin_state": n.AdminStateUp,
					"shared":      n.Shared,
				})
			}
			return true, nil
		})
		if err != nil {
			return shared.ToolError("failed to list networks: %v", err), nil
		}

		out, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

func listSubnetsHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.NetworkClient()
		if err != nil {
			return shared.ToolError("failed to get network client: %v", err), nil
		}

		opts := subnets.ListOpts{
			NetworkID: shared.StringParam(request, "network_id"),
		}

		var result []map[string]any
		err = subnets.List(client, opts).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
			subs, err := subnets.ExtractSubnets(page)
			if err != nil {
				return false, err
			}
			for _, s := range subs {
				result = append(result, map[string]any{
					"id":         s.ID,
					"name":       s.Name,
					"cidr":       s.CIDR,
					"gateway_ip": s.GatewayIP,
					"network_id": s.NetworkID,
					"ip_version": s.IPVersion,
					"dhcp":       s.EnableDHCP,
				})
			}
			return true, nil
		})
		if err != nil {
			return shared.ToolError("failed to list subnets: %v", err), nil
		}

		out, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

func listPortsHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.NetworkClient()
		if err != nil {
			return shared.ToolError("failed to get network client: %v", err), nil
		}

		opts := ports.ListOpts{
			NetworkID: shared.StringParam(request, "network_id"),
			DeviceID:  shared.StringParam(request, "device_id"),
			Status:    shared.StringParam(request, "status"),
		}

		var result []map[string]any
		err = ports.List(client, opts).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
			ps, err := ports.ExtractPorts(page)
			if err != nil {
				return false, err
			}
			for _, p := range ps {
				result = append(result, map[string]any{
					"id":           p.ID,
					"name":         p.Name,
					"status":       p.Status,
					"mac_address":  p.MACAddress,
					"fixed_ips":    p.FixedIPs,
					"device_id":    p.DeviceID,
					"device_owner": p.DeviceOwner,
					"network_id":   p.NetworkID,
				})
			}
			return true, nil
		})
		if err != nil {
			return shared.ToolError("failed to list ports: %v", err), nil
		}

		out, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

func listSecGroupsHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.NetworkClient()
		if err != nil {
			return shared.ToolError("failed to get network client: %v", err), nil
		}

		opts := groups.ListOpts{
			Name: shared.StringParam(request, "name"),
		}

		var result []map[string]any
		err = groups.List(client, opts).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
			sgs, err := groups.ExtractGroups(page)
			if err != nil {
				return false, err
			}
			for _, sg := range sgs {
				result = append(result, map[string]any{
					"id":          sg.ID,
					"name":        sg.Name,
					"description": sg.Description,
					"rules":       sg.Rules,
				})
			}
			return true, nil
		})
		if err != nil {
			return shared.ToolError("failed to list security groups: %v", err), nil
		}

		out, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}
