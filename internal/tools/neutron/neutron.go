// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

// Package neutron provides MCP tools for OpenStack Networking (Neutron) operations.
package neutron

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/extensions/agents"
	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/extensions/layer3/floatingips"
	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/extensions/layer3/routers"
	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/extensions/networkipavailabilities"
	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/extensions/security/groups"
	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/extensions/security/rules"
	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/extensions/trunks"
	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/networks"
	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/ports"
	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/subnets"
	"github.com/gophercloud/gophercloud/v2/pagination"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/sapcc/gophercloud-sapcc/v2/networking/v2/bgpvpn/interconnections"

	"github.com/notque/openstack-mcp-server/internal/auth"
	"github.com/notque/openstack-mcp-server/internal/tools/shared"
)

// Register adds all Neutron tools to the MCP server.
// When readOnly is true, mutating tools (create/delete operations) are not registered.
// When admin is true, admin-only tools (agents) are registered.
func Register(s *mcpserver.MCPServer, provider *auth.Provider, readOnly bool, admin bool) {
	s.AddTool(listNetworksTool, listNetworksHandler(provider))
	s.AddTool(listSubnetsTool, listSubnetsHandler(provider))
	s.AddTool(listPortsTool, listPortsHandler(provider))
	s.AddTool(listSecGroupsTool, listSecGroupsHandler(provider))
	s.AddTool(listRoutersTool, listRoutersHandler(provider))
	s.AddTool(listFloatingIPsTool, listFloatingIPsHandler(provider))
	s.AddTool(listTrunksTool, listTrunksHandler(provider))
	s.AddTool(listNetworkIPAvailabilitiesTool, listNetworkIPAvailabilitiesHandler(provider))
	s.AddTool(listBGPVPNInterconnectionsTool, listBGPVPNInterconnectionsHandler(provider))
	if !readOnly {
		s.AddTool(createSecGroupRuleTool, createSecGroupRuleHandler(provider))
		s.AddTool(deleteSecGroupRuleTool, deleteSecGroupRuleHandler(provider))
		s.AddTool(createFloatingIPTool, createFloatingIPHandler(provider))
		s.AddTool(deleteFloatingIPTool, deleteFloatingIPHandler(provider))
	}
	if admin {
		s.AddTool(listAgentsTool, listAgentsHandler(provider))
	}
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

		result := make([]map[string]any, 0)
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

		result := make([]map[string]any, 0)
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

		result := make([]map[string]any, 0)
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

		result := make([]map[string]any, 0)
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

var listRoutersTool = mcp.NewTool("neutron_list_routers",
	mcp.WithDescription("List routers in the current project. Returns router ID, name, status, external gateway info, and admin state."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithString("name", mcp.Description("Filter by router name")),
	mcp.WithString("status", mcp.Description("Filter by router status (ACTIVE, DOWN, BUILD, ERROR)")),
	mcp.WithNumber("limit", mcp.Description("Maximum number of routers to return")),
)

func listRoutersHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.NetworkClient()
		if err != nil {
			return shared.ToolError("failed to get network client: %v", err), nil
		}

		opts := routers.ListOpts{
			Name:   shared.StringParam(request, "name"),
			Status: shared.StringParam(request, "status"),
		}
		if limit := shared.NumberParam(request, "limit"); limit > 0 {
			opts.Limit = int(limit)
		}

		result := make([]map[string]any, 0)
		err = routers.List(client, opts).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
			rs, err := routers.ExtractRouters(page)
			if err != nil {
				return false, err
			}
			for _, r := range rs {
				result = append(result, map[string]any{
					"id":                    r.ID,
					"name":                  r.Name,
					"status":                r.Status,
					"admin_state_up":        r.AdminStateUp,
					"external_gateway_info": r.GatewayInfo,
					"distributed":           r.Distributed,
				})
			}
			return true, nil
		})
		if err != nil {
			return shared.ToolError("failed to list routers: %v", err), nil
		}

		out, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

var listFloatingIPsTool = mcp.NewTool("neutron_list_floating_ips",
	mcp.WithDescription("List floating IPs in the current project. Returns ID, floating IP address, fixed IP, port ID, router ID, and status."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithString("floating_ip_address", mcp.Description("Filter by floating IP address")),
	mcp.WithString("port_id", mcp.Description("Filter by port ID")),
	mcp.WithString("status", mcp.Description("Filter by floating IP status (ACTIVE, DOWN, ERROR)")),
	mcp.WithNumber("limit", mcp.Description("Maximum number of floating IPs to return")),
)

func listFloatingIPsHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.NetworkClient()
		if err != nil {
			return shared.ToolError("failed to get network client: %v", err), nil
		}

		opts := floatingips.ListOpts{
			FloatingIP: shared.StringParam(request, "floating_ip_address"),
			Status:     shared.StringParam(request, "status"),
		}
		if v := shared.StringParam(request, "port_id"); v != "" {
			if errResult := shared.ValidateUUID(v, "port_id"); errResult != nil {
				return errResult, nil
			}
			opts.PortID = v
		}
		if limit := shared.NumberParam(request, "limit"); limit > 0 {
			opts.Limit = int(limit)
		}

		result := make([]map[string]any, 0)
		err = floatingips.List(client, opts).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
			fips, err := floatingips.ExtractFloatingIPs(page)
			if err != nil {
				return false, err
			}
			for _, fip := range fips {
				result = append(result, map[string]any{
					"id":                  fip.ID,
					"floating_ip_address": fip.FloatingIP,
					"fixed_ip_address":    fip.FixedIP,
					"port_id":             fip.PortID,
					"router_id":           fip.RouterID,
					"status":              fip.Status,
					"floating_network_id": fip.FloatingNetworkID,
				})
			}
			return true, nil
		})
		if err != nil {
			return shared.ToolError("failed to list floating IPs: %v", err), nil
		}

		out, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

// --- Security Group Rule Write Tools ---

var createSecGroupRuleTool = mcp.NewTool("neutron_create_security_group_rule",
	mcp.WithDescription("Create a new security group rule. Requires confirmation."),
	mcp.WithDestructiveHintAnnotation(true),
	mcp.WithString("security_group_id", mcp.Required(), mcp.Description("The UUID of the security group to add the rule to")),
	mcp.WithString("direction", mcp.Required(), mcp.Description("Direction: 'ingress' or 'egress'")),
	mcp.WithString("protocol", mcp.Description("Protocol: 'tcp', 'udp', or 'icmp'")),
	mcp.WithNumber("port_range_min", mcp.Description("Minimum port number (or ICMP type)")),
	mcp.WithNumber("port_range_max", mcp.Description("Maximum port number (or ICMP code)")),
	mcp.WithString("remote_ip_prefix", mcp.Description("Remote IP prefix in CIDR notation (e.g., '10.0.0.0/24')")),
	mcp.WithString("remote_group_id", mcp.Description("Remote security group UUID (alternative to remote_ip_prefix)")),
	mcp.WithString("ethertype", mcp.Description("Ethertype: 'IPv4' (default) or 'IPv6'")),
	mcp.WithString("description", mcp.Description("Optional description of the rule")),
	mcp.WithBoolean("confirmed", mcp.Description("Set to true to execute. Without this, returns a preview of the action.")),
)

// dangerousPorts contains ports that should not be opened to 0.0.0.0/0 for ingress TCP.
var dangerousPorts = map[int]bool{
	22:   true, // SSH
	3389: true, // RDP
	3306: true, // MySQL
	5432: true, // PostgreSQL
}

func createSecGroupRuleHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.NetworkClient()
		if err != nil {
			return shared.ToolError("failed to get network client: %v", err), nil
		}

		secGroupID := shared.StringParam(request, "security_group_id")
		if secGroupID == "" {
			return shared.ToolError("security_group_id is required"), nil
		}
		if errResult := shared.ValidateUUID(secGroupID, "security_group_id"); errResult != nil {
			return errResult, nil
		}

		direction := shared.StringParam(request, "direction")
		if direction == "" {
			return shared.ToolError("direction is required"), nil
		}
		if direction != "ingress" && direction != "egress" {
			return shared.ToolError("direction must be 'ingress' or 'egress' (got: %q)", direction), nil
		}

		protocol := shared.StringParam(request, "protocol")
		portMin := int(shared.NumberParam(request, "port_range_min"))
		portMax := int(shared.NumberParam(request, "port_range_max"))
		remoteIP := shared.StringParam(request, "remote_ip_prefix")
		remoteGroupID := shared.StringParam(request, "remote_group_id")
		ethertype := shared.StringParam(request, "ethertype")
		description := shared.StringParam(request, "description")

		if ethertype == "" {
			ethertype = "IPv4"
		}

		// Validate optional UUID parameters.
		if remoteGroupID != "" {
			if errResult := shared.ValidateUUID(remoteGroupID, "remote_group_id"); errResult != nil {
				return errResult, nil
			}
		}

		// Security guardrail: reject rules that open dangerous ports to the world.
		// Checks both IPv4 (0.0.0.0/0) and IPv6 (::/0) world-open prefixes.
		isWorldOpen := remoteIP == "0.0.0.0/0" || remoteIP == "::/0"
		if isWorldOpen && direction == "ingress" && protocol == "tcp" && remoteGroupID == "" {
			// Reject if no port range specified (would open ALL ports).
			if portMin == 0 && portMax == 0 {
				return shared.ToolError(
					"refusing to create rule allowing unrestricted access to ALL TCP ports from %s. Specify port_range_min and port_range_max, or use a specific CIDR or remote_group_id.",
					remoteIP,
				), nil
			}
			// Reject if any dangerous port falls within the specified range.
			effectiveMax := portMax
			if effectiveMax == 0 {
				effectiveMax = portMin
			}
			for port := range dangerousPorts {
				if portMin <= port && port <= effectiveMax {
					return shared.ToolError(
						"refusing to create rule allowing unrestricted access to port %d from %s (port range %d-%d includes sensitive services). Use a specific CIDR or remote_group_id instead.",
						port, remoteIP, portMin, effectiveMax,
					), nil
				}
			}
		}

		// Build preview.
		preview := fmt.Sprintf("Will CREATE security group rule: %s %s port %d-%d from %s on group %s",
			direction, protocol, portMin, portMax, remoteIP, secGroupID)
		if result := shared.RequireConfirmation(request, preview); result != nil {
			return result, nil
		}

		// Build create options.
		createOpts := rules.CreateOpts{
			SecGroupID:     secGroupID,
			Direction:      rules.RuleDirection(direction),
			EtherType:      rules.RuleEtherType(ethertype),
			Protocol:       rules.RuleProtocol(protocol),
			RemoteIPPrefix: remoteIP,
			RemoteGroupID:  remoteGroupID,
			Description:    description,
		}
		if portMin > 0 {
			createOpts.PortRangeMin = portMin
		}
		if portMax > 0 {
			createOpts.PortRangeMax = portMax
		}

		rule, err := rules.Create(ctx, client, createOpts).Extract()
		if err != nil {
			return shared.ToolError("failed to create security group rule: %v", err), nil
		}

		safe := map[string]any{
			"id":                rule.ID,
			"direction":         rule.Direction,
			"protocol":          rule.Protocol,
			"port_range_min":    rule.PortRangeMin,
			"port_range_max":    rule.PortRangeMax,
			"remote_ip_prefix":  rule.RemoteIPPrefix,
			"remote_group_id":   rule.RemoteGroupID,
			"ethertype":         rule.EtherType,
			"security_group_id": rule.SecGroupID,
		}

		out, err := json.MarshalIndent(safe, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

var deleteSecGroupRuleTool = mcp.NewTool("neutron_delete_security_group_rule",
	mcp.WithDescription("Delete a security group rule. Requires confirmation."),
	mcp.WithDestructiveHintAnnotation(true),
	mcp.WithString("rule_id", mcp.Required(), mcp.Description("The UUID of the security group rule to delete")),
	mcp.WithBoolean("confirmed", mcp.Description("Set to true to execute. Without this, returns a preview of the action.")),
)

func deleteSecGroupRuleHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.NetworkClient()
		if err != nil {
			return shared.ToolError("failed to get network client: %v", err), nil
		}

		ruleID := shared.StringParam(request, "rule_id")
		if ruleID == "" {
			return shared.ToolError("rule_id is required"), nil
		}
		if errResult := shared.ValidateUUID(ruleID, "rule_id"); errResult != nil {
			return errResult, nil
		}

		// Fetch rule for preview.
		rule, err := rules.Get(ctx, client, ruleID).Extract()
		if err != nil {
			return shared.ToolError("failed to get security group rule %s: %v", ruleID, err), nil
		}

		preview := fmt.Sprintf("Will DELETE security group rule: %s %s port %d-%d from %s (group %s)",
			rule.Direction, rule.Protocol, rule.PortRangeMin, rule.PortRangeMax, rule.RemoteIPPrefix, rule.SecGroupID)
		if result := shared.RequireConfirmation(request, preview); result != nil {
			return result, nil
		}

		if err := rules.Delete(ctx, client, ruleID).ExtractErr(); err != nil {
			return shared.ToolError("failed to delete security group rule %s: %v", ruleID, err), nil
		}

		return shared.ToolResult("Successfully deleted security group rule " + ruleID), nil
	}
}

// --- Trunk Tools ---

var listTrunksTool = mcp.NewTool("neutron_list_trunks",
	mcp.WithDescription("List trunk ports with sub-ports. Returns trunk ID, name, port ID, status, sub-ports, and admin state."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithString("name", mcp.Description("Filter by trunk name")),
	mcp.WithString("port_id", mcp.Description("Filter by parent port ID")),
)

func listTrunksHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.NetworkClient()
		if err != nil {
			return shared.ToolError("failed to get network client: %v", err), nil
		}

		opts := trunks.ListOpts{
			Name: shared.StringParam(request, "name"),
		}
		if v := shared.StringParam(request, "port_id"); v != "" {
			if errResult := shared.ValidateUUID(v, "port_id"); errResult != nil {
				return errResult, nil
			}
			opts.PortID = v
		}

		result := make([]map[string]any, 0)
		err = trunks.List(client, opts).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
			ts, err := trunks.ExtractTrunks(page)
			if err != nil {
				return false, err
			}
			for _, t := range ts {
				result = append(result, map[string]any{
					"id":             t.ID,
					"name":           t.Name,
					"port_id":        t.PortID,
					"status":         t.Status,
					"sub_ports":      t.Subports,
					"tenant_id":      t.TenantID,
					"admin_state_up": t.AdminStateUp,
				})
			}
			return true, nil
		})
		if err != nil {
			return shared.ToolError("failed to list trunks: %v", err), nil
		}

		out, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

// --- Network IP Availability Tools ---

var listNetworkIPAvailabilitiesTool = mcp.NewTool("neutron_list_network_ip_availabilities",
	mcp.WithDescription("Show IP usage per network. Returns network ID, name, total IPs, used IPs, and subnet IP availabilities."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithString("network_id", mcp.Description("Filter by network UUID")),
	mcp.WithString("network_name", mcp.Description("Filter by network name")),
)

func listNetworkIPAvailabilitiesHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.NetworkClient()
		if err != nil {
			return shared.ToolError("failed to get network client: %v", err), nil
		}

		opts := networkipavailabilities.ListOpts{
			NetworkName: shared.StringParam(request, "network_name"),
		}
		if v := shared.StringParam(request, "network_id"); v != "" {
			if errResult := shared.ValidateUUID(v, "network_id"); errResult != nil {
				return errResult, nil
			}
			opts.NetworkID = v
		}

		result := make([]map[string]any, 0)
		err = networkipavailabilities.List(client, opts).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
			avails, err := networkipavailabilities.ExtractNetworkIPAvailabilities(page)
			if err != nil {
				return false, err
			}
			for _, a := range avails {
				result = append(result, map[string]any{
					"network_id":               a.NetworkID,
					"network_name":             a.NetworkName,
					"total_ips":                a.TotalIPs,
					"used_ips":                 a.UsedIPs,
					"subnet_ip_availabilities": a.SubnetIPAvailabilities,
				})
			}
			return true, nil
		})
		if err != nil {
			return shared.ToolError("failed to list network IP availabilities: %v", err), nil
		}

		out, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

// --- Floating IP Write Tools ---

var createFloatingIPTool = mcp.NewTool("neutron_create_floating_ip",
	mcp.WithDescription("Create/allocate a floating IP from an external network. Requires confirmation."),
	mcp.WithDestructiveHintAnnotation(true),
	mcp.WithString("floating_network_id", mcp.Required(), mcp.Description("The UUID of the external network to allocate the floating IP from")),
	mcp.WithString("port_id", mcp.Description("The UUID of the internal port to associate the floating IP with")),
	mcp.WithString("subnet_id", mcp.Description("The UUID of the subnet for the floating IP")),
	mcp.WithBoolean("confirmed", mcp.Description("Set to true to execute. Without this, returns a preview of the action.")),
)

func createFloatingIPHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.NetworkClient()
		if err != nil {
			return shared.ToolError("failed to get network client: %v", err), nil
		}

		floatingNetworkID := shared.StringParam(request, "floating_network_id")
		if floatingNetworkID == "" {
			return shared.ToolError("floating_network_id is required"), nil
		}
		if errResult := shared.ValidateUUID(floatingNetworkID, "floating_network_id"); errResult != nil {
			return errResult, nil
		}

		portID := shared.StringParam(request, "port_id")
		if portID != "" {
			if errResult := shared.ValidateUUID(portID, "port_id"); errResult != nil {
				return errResult, nil
			}
		}

		subnetID := shared.StringParam(request, "subnet_id")
		if subnetID != "" {
			if errResult := shared.ValidateUUID(subnetID, "subnet_id"); errResult != nil {
				return errResult, nil
			}
		}

		preview := fmt.Sprintf("Will ALLOCATE floating IP from network %s", floatingNetworkID)
		if portID != "" {
			preview += fmt.Sprintf(" (associated with port %s)", portID)
		}
		if result := shared.RequireConfirmation(request, preview); result != nil {
			return result, nil
		}

		createOpts := floatingips.CreateOpts{
			FloatingNetworkID: floatingNetworkID,
			PortID:            portID,
			SubnetID:          subnetID,
		}

		fip, err := floatingips.Create(ctx, client, createOpts).Extract()
		if err != nil {
			return shared.ToolError("failed to create floating IP: %v", err), nil
		}

		safe := map[string]any{
			"id":                  fip.ID,
			"floating_ip_address": fip.FloatingIP,
			"floating_network_id": fip.FloatingNetworkID,
			"port_id":             fip.PortID,
			"status":              fip.Status,
		}

		out, err := json.MarshalIndent(safe, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

var deleteFloatingIPTool = mcp.NewTool("neutron_delete_floating_ip",
	mcp.WithDescription("Release/delete a floating IP. Requires confirmation."),
	mcp.WithDestructiveHintAnnotation(true),
	mcp.WithString("floating_ip_id", mcp.Required(), mcp.Description("The UUID of the floating IP to delete")),
	mcp.WithBoolean("confirmed", mcp.Description("Set to true to execute. Without this, returns a preview of the action.")),
)

func deleteFloatingIPHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.NetworkClient()
		if err != nil {
			return shared.ToolError("failed to get network client: %v", err), nil
		}

		fipID := shared.StringParam(request, "floating_ip_id")
		if fipID == "" {
			return shared.ToolError("floating_ip_id is required"), nil
		}
		if errResult := shared.ValidateUUID(fipID, "floating_ip_id"); errResult != nil {
			return errResult, nil
		}

		// Fetch floating IP for preview.
		fip, err := floatingips.Get(ctx, client, fipID).Extract()
		if err != nil {
			return shared.ToolError("failed to get floating IP %s: %v", fipID, err), nil
		}

		preview := fmt.Sprintf("Will DELETE floating IP %s (%s)", fip.FloatingIP, fip.ID)
		if result := shared.RequireConfirmation(request, preview); result != nil {
			return result, nil
		}

		if err := floatingips.Delete(ctx, client, fipID).ExtractErr(); err != nil {
			return shared.ToolError("failed to delete floating IP %s: %v", fipID, err), nil
		}

		return shared.ToolResult(fmt.Sprintf("Successfully deleted floating IP %s (%s)", fip.FloatingIP, fipID)), nil
	}
}

// --- Admin Tools ---

var listAgentsTool = mcp.NewTool("neutron_list_agents",
	mcp.WithDescription("[Admin] List neutron agents. Returns agent ID, type, binary, host, alive status, and heartbeat."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithString("agent_type", mcp.Description("Filter by agent type (e.g., 'Open vSwitch agent', 'DHCP agent', 'L3 agent')")),
	mcp.WithString("host", mcp.Description("Filter by host name")),
	mcp.WithString("alive", mcp.Description("Filter by alive status ('true' or 'false')")),
)

func listAgentsHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.NetworkClient()
		if err != nil {
			return shared.ToolError("failed to get network client: %v", err), nil
		}

		opts := agents.ListOpts{
			AgentType: shared.StringParam(request, "agent_type"),
			Host:      shared.StringParam(request, "host"),
		}
		if v := shared.StringParam(request, "alive"); v != "" {
			switch v {
			case "true":
				alive := true
				opts.Alive = &alive
			case "false":
				alive := false
				opts.Alive = &alive
			default:
				return shared.ToolError("alive must be 'true' or 'false' (got: %q)", v), nil
			}
		}

		result := make([]map[string]any, 0)
		err = agents.List(client, opts).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
			as, err := agents.ExtractAgents(page)
			if err != nil {
				return false, err
			}
			for _, a := range as {
				result = append(result, map[string]any{
					"id":                  a.ID,
					"agent_type":          a.AgentType,
					"binary":              a.Binary,
					"host":                a.Host,
					"alive":               a.Alive,
					"admin_state_up":      a.AdminStateUp,
					"topic":               a.Topic,
					"heartbeat_timestamp": a.HeartbeatTimestamp,
				})
			}
			return true, nil
		})
		if err != nil {
			return shared.ToolError("failed to list agents: %v", err), nil
		}

		out, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

// --- BGP VPN Interconnection Tools (SAP CC) ---

var listBGPVPNInterconnectionsTool = mcp.NewTool("neutron_list_bgpvpn_interconnections",
	mcp.WithDescription("List BGP VPN interconnections (SAP CC extension). Returns interconnection ID, name, type, state, and resource details."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithString("name", mcp.Description("Filter by interconnection name")),
	mcp.WithString("state", mcp.Description("Filter by interconnection state")),
	mcp.WithString("project_id", mcp.Description("Filter by project ID")),
)

func listBGPVPNInterconnectionsHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.NetworkClient()
		if err != nil {
			return shared.ToolError("failed to get network client: %v", err), nil
		}

		opts := interconnections.ListOpts{}
		if v := shared.StringParam(request, "name"); v != "" {
			opts.Name = []string{v}
		}
		if v := shared.StringParam(request, "state"); v != "" {
			opts.State = []string{v}
		}
		if v := shared.StringParam(request, "project_id"); v != "" {
			if errResult := shared.ValidateUUID(v, "project_id"); errResult != nil {
				return errResult, nil
			}
			opts.ProjectID = []string{v}
		}

		result := make([]map[string]any, 0)
		err = interconnections.List(client, opts).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
			ics, err := interconnections.ExtractInterconnections(page)
			if err != nil {
				return false, err
			}
			for _, ic := range ics {
				result = append(result, map[string]any{
					"id":                 ic.ID,
					"name":               ic.Name,
					"project_id":         ic.ProjectID,
					"type":               ic.Type,
					"state":              ic.State,
					"local_resource_id":  ic.LocalResourceID,
					"remote_resource_id": ic.RemoteResourceID,
					"remote_region":      ic.RemoteRegion,
				})
			}
			return true, nil
		})
		if err != nil {
			return shared.ToolError("failed to list BGP VPN interconnections: %v", err), nil
		}

		out, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}
