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
	mcp.WithNumber("limit", mcp.Description("Maximum number of networks to return")),
)

var listSubnetsTool = mcp.NewTool("neutron_list_subnets",
	mcp.WithDescription("List subnets in the current project. Returns subnet ID, name, CIDR, gateway, and network ID."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithString("network_id", mcp.Description("Filter by network ID")),
	mcp.WithString("name", mcp.Description("Filter by subnet name")),
	mcp.WithString("cidr", mcp.Description("Filter by CIDR block (e.g., '10.0.1.0/24')")),
	mcp.WithNumber("ip_version", mcp.Description("Filter by IP version (4 or 6)")),
	mcp.WithNumber("limit", mcp.Description("Maximum number of subnets to return")),
)

var listPortsTool = mcp.NewTool("neutron_list_ports",
	mcp.WithDescription("List ports in the current project. Returns port ID, name, status, MAC, fixed IPs, and device owner."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithString("network_id", mcp.Description("Filter by network ID")),
	mcp.WithString("device_id", mcp.Description("Filter by device (server) ID")),
	mcp.WithString("status", mcp.Description("Filter by port status (ACTIVE, DOWN, BUILD)")),
	mcp.WithString("device_owner", mcp.Description("Filter by device owner (e.g., 'compute:nova', 'network:router_interface', 'network:dhcp')")),
	mcp.WithString("mac_address", mcp.Description("Filter by MAC address")),
	mcp.WithNumber("limit", mcp.Description("Maximum number of ports to return")),
)

var listSecGroupsTool = mcp.NewTool("neutron_list_security_groups",
	mcp.WithDescription("List security groups in the current project with their rules."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithString("name", mcp.Description("Filter by security group name")),
	mcp.WithNumber("limit", mcp.Description("Maximum number of security groups to return")),
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
		if limit := shared.NumberParam(request, "limit"); limit > 0 {
			opts.Limit = int(limit)
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
			Name:      shared.StringParam(request, "name"),
			CIDR:      shared.StringParam(request, "cidr"),
			IPVersion: int(shared.NumberParam(request, "ip_version")),
		}
		if limit := shared.NumberParam(request, "limit"); limit > 0 {
			opts.Limit = int(limit)
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
			NetworkID:   shared.StringParam(request, "network_id"),
			DeviceID:    shared.StringParam(request, "device_id"),
			Status:      shared.StringParam(request, "status"),
			DeviceOwner: shared.StringParam(request, "device_owner"),
			MACAddress:  shared.StringParam(request, "mac_address"),
		}
		if limit := shared.NumberParam(request, "limit"); limit > 0 {
			opts.Limit = int(limit)
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
		if limit := shared.NumberParam(request, "limit"); limit > 0 {
			opts.Limit = int(limit)
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
