// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

// Package octavia provides MCP tools for OpenStack Load Balancer (Octavia) operations.
package octavia

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gophercloud/gophercloud/v2/openstack/loadbalancer/v2/amphorae"
	"github.com/gophercloud/gophercloud/v2/openstack/loadbalancer/v2/l7policies"
	"github.com/gophercloud/gophercloud/v2/openstack/loadbalancer/v2/listeners"
	"github.com/gophercloud/gophercloud/v2/openstack/loadbalancer/v2/loadbalancers"
	"github.com/gophercloud/gophercloud/v2/openstack/loadbalancer/v2/monitors"
	"github.com/gophercloud/gophercloud/v2/openstack/loadbalancer/v2/pools"
	"github.com/gophercloud/gophercloud/v2/pagination"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/notque/openstack-mcp-server/internal/auth"
	"github.com/notque/openstack-mcp-server/internal/tools/shared"
)

// Register adds all Octavia tools to the MCP server.
// When readOnly is true, mutating tools (create/delete load balancers) are not registered.
// When admin is true, admin-only tools (amphorae) are registered.
func Register(s *mcpserver.MCPServer, provider *auth.Provider, readOnly, admin bool) {
	s.AddTool(listLoadbalancersTool, listLoadbalancersHandler(provider))
	s.AddTool(getLoadbalancerTool, getLoadbalancerHandler(provider))
	s.AddTool(listListenersTool, listListenersHandler(provider))
	s.AddTool(listPoolsTool, listPoolsHandler(provider))
	s.AddTool(listMembersTool, listMembersHandler(provider))
	s.AddTool(listHealthmonitorsTool, listHealthmonitorsHandler(provider))
	s.AddTool(listL7policiesTool, listL7policiesHandler(provider))
	s.AddTool(listL7RulesTool, listL7RulesHandler(provider))

	if admin {
		s.AddTool(listAmphoraeTool, listAmphoraeHandler(provider))
	}

	if !readOnly {
		s.AddTool(createLoadbalancerTool, createLoadbalancerHandler(provider))
		s.AddTool(deleteLoadbalancerTool, deleteLoadbalancerHandler(provider))
	}
}

// fieldProvisioningStatus is the JSON field name used across multiple response maps.
const fieldProvisioningStatus = "provisioning_status"

// --- Load Balancers ---

var listLoadbalancersTool = mcp.NewTool("octavia_list_loadbalancers",
	mcp.WithDescription("List load balancers in the current project. Returns ID, name, status, VIP address, and provider."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithString("name", mcp.Description("Filter by load balancer name")),
	mcp.WithString("provisioning_status", mcp.Description("Filter by provisioning status (ACTIVE, PENDING_CREATE, ERROR)")),
	mcp.WithString("vip_address", mcp.Description("Filter by virtual IP address")),
	mcp.WithString("operating_status", mcp.Description("Filter by operating status (ONLINE, OFFLINE, DEGRADED, ERROR, NO_MONITOR)")),
	mcp.WithString("vip_subnet_id", mcp.Description("Filter by VIP subnet UUID")),
	mcp.WithString("provider", mcp.Description("Filter by load balancer provider (e.g., 'amphora', 'ovn')")),
)

var getLoadbalancerTool = mcp.NewTool("octavia_get_loadbalancer",
	mcp.WithDescription("Get detailed information about a specific load balancer."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithString("loadbalancer_id", mcp.Required(), mcp.Description("The UUID of the load balancer to retrieve")),
)

func listLoadbalancersHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.LoadBalancerClient()
		if err != nil {
			return shared.ToolError("failed to get load balancer client: %v", err), nil
		}

		opts := loadbalancers.ListOpts{}
		if v := shared.StringParam(request, "name"); v != "" {
			opts.Name = v
		}
		if v := shared.StringParam(request, "provisioning_status"); v != "" {
			opts.ProvisioningStatus = v
		}
		if v := shared.StringParam(request, "vip_address"); v != "" {
			opts.VipAddress = v
		}
		if v := shared.StringParam(request, "operating_status"); v != "" {
			opts.OperatingStatus = v
		}
		if v := shared.StringParam(request, "vip_subnet_id"); v != "" {
			opts.VipSubnetID = v
		}
		if v := shared.StringParam(request, "provider"); v != "" {
			opts.Provider = v
		}

		var result []map[string]any
		err = loadbalancers.List(client, opts).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
			lbs, err := loadbalancers.ExtractLoadBalancers(page)
			if err != nil {
				return false, err
			}
			for _, lb := range lbs {
				result = append(result, map[string]any{
					"id":                    lb.ID,
					"name":                  lb.Name,
					fieldProvisioningStatus: lb.ProvisioningStatus,
					"operating_status":      lb.OperatingStatus,
					"vip_address":           lb.VipAddress,
					"vip_subnet_id":         lb.VipSubnetID,
					"provider":              lb.Provider,
					"created_at":            lb.CreatedAt,
				})
			}
			return true, nil
		})
		if err != nil {
			return shared.ToolError("failed to list load balancers: %v", err), nil
		}

		out, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

func getLoadbalancerHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.LoadBalancerClient()
		if err != nil {
			return shared.ToolError("failed to get load balancer client: %v", err), nil
		}

		lbID := shared.StringParam(request, "loadbalancer_id")
		if lbID == "" {
			return shared.ToolError("loadbalancer_id is required"), nil
		}
		if errResult := shared.ValidateUUID(lbID, "loadbalancer_id"); errResult != nil {
			return errResult, nil
		}

		lb, err := loadbalancers.Get(ctx, client, lbID).Extract()
		if err != nil {
			return shared.ToolError("failed to get load balancer %s: %v", lbID, err), nil
		}

		out, err := json.MarshalIndent(lb, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

// --- Listeners ---

var listListenersTool = mcp.NewTool("octavia_list_listeners",
	mcp.WithDescription("List load balancer listeners. Returns ID, name, protocol, port, and associated load balancers."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithString("name", mcp.Description("Filter by listener name")),
	mcp.WithString("protocol", mcp.Description("Filter by protocol (TCP, HTTP, HTTPS, TERMINATED_HTTPS, UDP, SCTP)")),
	mcp.WithString("loadbalancer_id", mcp.Description("Filter by load balancer UUID")),
	mcp.WithNumber("protocol_port", mcp.Description("Filter by protocol port number (e.g., 443, 80, 8080)")),
)

func listListenersHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.LoadBalancerClient()
		if err != nil {
			return shared.ToolError("failed to get load balancer client: %v", err), nil
		}

		opts := listeners.ListOpts{}
		if v := shared.StringParam(request, "name"); v != "" {
			opts.Name = v
		}
		if v := shared.StringParam(request, "protocol"); v != "" {
			opts.Protocol = v
		}
		if v := shared.StringParam(request, "loadbalancer_id"); v != "" {
			opts.LoadbalancerID = v
		}
		if v := shared.NumberParam(request, "protocol_port"); v != 0 {
			opts.ProtocolPort = int(v)
		}

		var result []map[string]any
		err = listeners.List(client, opts).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
			allListeners, err := listeners.ExtractListeners(page)
			if err != nil {
				return false, err
			}
			for _, l := range allListeners {
				lbIDs := make([]string, len(l.Loadbalancers))
				for i, lb := range l.Loadbalancers {
					lbIDs[i] = lb.ID
				}
				result = append(result, map[string]any{
					"id":                    l.ID,
					"name":                  l.Name,
					"protocol":              l.Protocol,
					"protocol_port":         l.ProtocolPort,
					"default_pool_id":       l.DefaultPoolID,
					fieldProvisioningStatus: l.ProvisioningStatus,
					"loadbalancers":         lbIDs,
				})
			}
			return true, nil
		})
		if err != nil {
			return shared.ToolError("failed to list listeners: %v", err), nil
		}

		out, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

// --- Pools ---

var listPoolsTool = mcp.NewTool("octavia_list_pools",
	mcp.WithDescription("List load balancer pools. Returns ID, name, protocol, LB algorithm, and status."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithString("name", mcp.Description("Filter by pool name")),
	mcp.WithString("protocol", mcp.Description("Filter by protocol (TCP, HTTP, HTTPS, PROXY, UDP, SCTP)")),
	mcp.WithString("loadbalancer_id", mcp.Description("Filter by load balancer UUID")),
	mcp.WithString("lb_algorithm", mcp.Description("Filter by load balancing algorithm (ROUND_ROBIN, LEAST_CONNECTIONS, SOURCE_IP)")),
)

func listPoolsHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.LoadBalancerClient()
		if err != nil {
			return shared.ToolError("failed to get load balancer client: %v", err), nil
		}

		opts := pools.ListOpts{}
		if v := shared.StringParam(request, "name"); v != "" {
			opts.Name = v
		}
		if v := shared.StringParam(request, "protocol"); v != "" {
			opts.Protocol = v
		}
		if v := shared.StringParam(request, "loadbalancer_id"); v != "" {
			opts.LoadbalancerID = v
		}
		if v := shared.StringParam(request, "lb_algorithm"); v != "" {
			opts.LBMethod = v
		}

		var result []map[string]any
		err = pools.List(client, opts).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
			allPools, err := pools.ExtractPools(page)
			if err != nil {
				return false, err
			}
			for _, p := range allPools {
				lbIDs := make([]string, len(p.Loadbalancers))
				for i, lb := range p.Loadbalancers {
					lbIDs[i] = lb.ID
				}
				result = append(result, map[string]any{
					"id":                    p.ID,
					"name":                  p.Name,
					"protocol":              p.Protocol,
					"lb_method":             p.LBMethod,
					fieldProvisioningStatus: p.ProvisioningStatus,
					"operating_status":      p.OperatingStatus,
					"loadbalancers":         lbIDs,
				})
			}
			return true, nil
		})
		if err != nil {
			return shared.ToolError("failed to list pools: %v", err), nil
		}

		out, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

// --- Members ---

var listMembersTool = mcp.NewTool("octavia_list_members",
	mcp.WithDescription("List members in a load balancer pool. Returns member ID, name, address, protocol port, weight, and operating status."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithString("pool_id", mcp.Required(), mcp.Description("The UUID of the pool to list members for")),
	mcp.WithString("name", mcp.Description("Filter by member name")),
	mcp.WithString("address", mcp.Description("Filter by member address")),
)

func listMembersHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.LoadBalancerClient()
		if err != nil {
			return shared.ToolError("failed to get load balancer client: %v", err), nil
		}

		poolID := shared.StringParam(request, "pool_id")
		if poolID == "" {
			return shared.ToolError("pool_id is required"), nil
		}
		if errResult := shared.ValidateUUID(poolID, "pool_id"); errResult != nil {
			return errResult, nil
		}

		opts := pools.ListMembersOpts{}
		if v := shared.StringParam(request, "name"); v != "" {
			opts.Name = v
		}
		if v := shared.StringParam(request, "address"); v != "" {
			opts.Address = v
		}

		var result []map[string]any
		err = pools.ListMembers(client, poolID, opts).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
			allMembers, err := pools.ExtractMembers(page)
			if err != nil {
				return false, err
			}
			for _, m := range allMembers {
				result = append(result, map[string]any{
					"id":               m.ID,
					"name":             m.Name,
					"address":          m.Address,
					"protocol_port":    m.ProtocolPort,
					"weight":           m.Weight,
					"operating_status": m.OperatingStatus,
					"admin_state_up":   m.AdminStateUp,
					"subnet_id":        m.SubnetID,
				})
			}
			return true, nil
		})
		if err != nil {
			return shared.ToolError("failed to list members for pool %s: %v", poolID, err), nil
		}

		out, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

// --- Health Monitors ---

var listHealthmonitorsTool = mcp.NewTool("octavia_list_healthmonitors",
	mcp.WithDescription("List health monitors in the current project. Returns monitor ID, name, type, delay, timeout, max retries, pool ID, and operating status."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithString("pool_id", mcp.Description("Filter by pool UUID")),
	mcp.WithString("type", mcp.Description("Filter by monitor type (HTTP, HTTPS, PING, TCP, TLS-HELLO, UDP-CONNECT)")),
)

func listHealthmonitorsHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.LoadBalancerClient()
		if err != nil {
			return shared.ToolError("failed to get load balancer client: %v", err), nil
		}

		opts := monitors.ListOpts{}
		if v := shared.StringParam(request, "pool_id"); v != "" {
			if errResult := shared.ValidateUUID(v, "pool_id"); errResult != nil {
				return errResult, nil
			}
			opts.PoolID = v
		}
		if v := shared.StringParam(request, "type"); v != "" {
			opts.Type = v
		}

		var result []map[string]any
		err = monitors.List(client, opts).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
			allMonitors, err := monitors.ExtractMonitors(page)
			if err != nil {
				return false, err
			}
			for _, m := range allMonitors {
				poolIDs := make([]string, len(m.Pools))
				for i, p := range m.Pools {
					poolIDs[i] = p.ID
				}
				result = append(result, map[string]any{
					"id":               m.ID,
					"name":             m.Name,
					"type":             m.Type,
					"delay":            m.Delay,
					"timeout":          m.Timeout,
					"max_retries":      m.MaxRetries,
					"pools":            poolIDs,
					"operating_status": m.OperatingStatus,
					"admin_state_up":   m.AdminStateUp,
				})
			}
			return true, nil
		})
		if err != nil {
			return shared.ToolError("failed to list health monitors: %v", err), nil
		}

		out, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

// --- L7 Policies ---

var listL7policiesTool = mcp.NewTool("octavia_list_l7policies",
	mcp.WithDescription("List L7 policies for load balancer listeners. Returns ID, name, action, redirect info, position, and status."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithString("listener_id", mcp.Description("Filter by listener UUID")),
	mcp.WithString("name", mcp.Description("Filter by L7 policy name")),
)

func listL7policiesHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.LoadBalancerClient()
		if err != nil {
			return shared.ToolError("failed to get load balancer client: %v", err), nil
		}

		opts := l7policies.ListOpts{}
		if v := shared.StringParam(request, "listener_id"); v != "" {
			if errResult := shared.ValidateUUID(v, "listener_id"); errResult != nil {
				return errResult, nil
			}
			opts.ListenerID = v
		}
		if v := shared.StringParam(request, "name"); v != "" {
			opts.Name = v
		}

		result := make([]map[string]any, 0)
		err = l7policies.List(client, opts).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
			allPolicies, err := l7policies.ExtractL7Policies(page)
			if err != nil {
				return false, err
			}
			for _, p := range allPolicies {
				result = append(result, map[string]any{
					"id":                    p.ID,
					"name":                  p.Name,
					"action":                p.Action,
					"redirect_pool_id":      p.RedirectPoolID,
					"redirect_url":          p.RedirectURL,
					"position":              p.Position,
					fieldProvisioningStatus: p.ProvisioningStatus,
					"operating_status":      p.OperatingStatus,
				})
			}
			return true, nil
		})
		if err != nil {
			return shared.ToolError("failed to list L7 policies: %v", err), nil
		}

		out, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

// --- L7 Rules ---

var listL7RulesTool = mcp.NewTool("octavia_list_l7rules",
	mcp.WithDescription("List L7 rules for a specific policy. Returns rule ID, type, compare type, key, value, invert, and status."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithString("l7policy_id", mcp.Required(), mcp.Description("The UUID of the L7 policy to list rules for")),
)

func listL7RulesHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.LoadBalancerClient()
		if err != nil {
			return shared.ToolError("failed to get load balancer client: %v", err), nil
		}

		policyID := shared.StringParam(request, "l7policy_id")
		if policyID == "" {
			return shared.ToolError("l7policy_id is required"), nil
		}
		if errResult := shared.ValidateUUID(policyID, "l7policy_id"); errResult != nil {
			return errResult, nil
		}

		result := make([]map[string]any, 0)
		err = l7policies.ListRules(client, policyID, nil).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
			allRules, err := l7policies.ExtractRules(page)
			if err != nil {
				return false, err
			}
			for _, r := range allRules {
				result = append(result, map[string]any{
					"id":                    r.ID,
					"type":                  r.RuleType,
					"compare_type":          r.CompareType,
					"key":                   r.Key,
					"value":                 r.Value,
					"invert":                r.Invert,
					"operating_status":      r.OperatingStatus,
					fieldProvisioningStatus: r.ProvisioningStatus,
				})
			}
			return true, nil
		})
		if err != nil {
			return shared.ToolError("failed to list L7 rules for policy %s: %v", policyID, err), nil
		}

		out, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

// --- Admin tools ---

var listAmphoraeTool = mcp.NewTool("octavia_list_amphorae",
	mcp.WithDescription("[Admin] List amphora instances. Requires admin role."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithString("loadbalancer_id", mcp.Description("Filter by load balancer UUID")),
	mcp.WithString("status", mcp.Description("Filter by amphora status")),
)

func listAmphoraeHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.LoadBalancerClient()
		if err != nil {
			return shared.ToolError("failed to get load balancer client: %v", err), nil
		}

		opts := amphorae.ListOpts{}
		if v := shared.StringParam(request, "loadbalancer_id"); v != "" {
			if errResult := shared.ValidateUUID(v, "loadbalancer_id"); errResult != nil {
				return errResult, nil
			}
			opts.LoadbalancerID = v
		}
		if v := shared.StringParam(request, "status"); v != "" {
			opts.Status = v
		}

		result := make([]map[string]any, 0)
		err = amphorae.List(client, opts).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
			allAmphorae, err := amphorae.ExtractAmphorae(page)
			if err != nil {
				return false, err
			}
			for _, a := range allAmphorae {
				result = append(result, map[string]any{
					"id":              a.ID,
					"loadbalancer_id": a.LoadbalancerID,
					"status":          a.Status,
					"role":            a.Role,
					"lb_network_ip":   a.LBNetworkIP,
					"ha_port_id":      a.HAPortID,
					"compute_id":      a.ComputeID,
					"cert_expiration": a.CertExpiration,
				})
			}
			return true, nil
		})
		if err != nil {
			return shared.ToolError("failed to list amphorae: %v", err), nil
		}

		out, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

// --- Write Tools ---

var createLoadbalancerTool = mcp.NewTool("octavia_create_loadbalancer",
	mcp.WithDescription("Create a new load balancer on a specified subnet."),
	mcp.WithDestructiveHintAnnotation(true),
	mcp.WithString("name", mcp.Required(), mcp.Description("Name for the load balancer")),
	mcp.WithString("vip_subnet_id", mcp.Required(), mcp.Description("The UUID of the subnet for the VIP address")),
	mcp.WithString("description", mcp.Description("Human-readable description")),
	mcp.WithBoolean("confirmed", mcp.Description("Set to true to execute. Without this, returns a preview of the action.")),
)

var deleteLoadbalancerTool = mcp.NewTool("octavia_delete_loadbalancer",
	mcp.WithDescription("Delete a load balancer. Optionally cascade-deletes all child resources (listeners, pools, members, health monitors)."),
	mcp.WithDestructiveHintAnnotation(true),
	mcp.WithString("loadbalancer_id", mcp.Required(), mcp.Description("The UUID of the load balancer to delete")),
	mcp.WithBoolean("cascade", mcp.Description("If true, deletes all associated listeners, pools, members, and health monitors")),
	mcp.WithBoolean("confirmed", mcp.Description("Set to true to execute. Without this, returns a preview of the action.")),
)

func createLoadbalancerHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.LoadBalancerClient()
		if err != nil {
			return shared.ToolError("failed to get load balancer client: %v", err), nil
		}

		name := shared.StringParam(request, "name")
		if name == "" {
			return shared.ToolError("name is required"), nil
		}

		vipSubnetID := shared.StringParam(request, "vip_subnet_id")
		if vipSubnetID == "" {
			return shared.ToolError("vip_subnet_id is required"), nil
		}
		if errResult := shared.ValidateUUID(vipSubnetID, "vip_subnet_id"); errResult != nil {
			return errResult, nil
		}

		preview := fmt.Sprintf("Will CREATE load balancer '%s' on subnet %s", name, vipSubnetID)
		if result := shared.RequireConfirmation(request, preview); result != nil {
			return result, nil
		}

		createOpts := loadbalancers.CreateOpts{
			Name:        name,
			VipSubnetID: vipSubnetID,
			Description: shared.StringParam(request, "description"),
		}

		lb, err := loadbalancers.Create(ctx, client, createOpts).Extract()
		if err != nil {
			return shared.ToolError("failed to create load balancer: %v", err), nil
		}

		lbResult := map[string]any{
			"id":                    lb.ID,
			"name":                  lb.Name,
			"vip_address":           lb.VipAddress,
			"vip_subnet_id":         lb.VipSubnetID,
			"operating_status":      lb.OperatingStatus,
			fieldProvisioningStatus: lb.ProvisioningStatus,
			"provider":              lb.Provider,
		}

		out, err := json.MarshalIndent(lbResult, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

func deleteLoadbalancerHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.LoadBalancerClient()
		if err != nil {
			return shared.ToolError("failed to get load balancer client: %v", err), nil
		}

		lbID := shared.StringParam(request, "loadbalancer_id")
		if lbID == "" {
			return shared.ToolError("loadbalancer_id is required"), nil
		}
		if errResult := shared.ValidateUUID(lbID, "loadbalancer_id"); errResult != nil {
			return errResult, nil
		}

		cascade := shared.BoolParam(request, "cascade")

		// Always fetch the LB to verify state and build preview.
		lb, err := loadbalancers.Get(ctx, client, lbID).Extract()
		if err != nil {
			return shared.ToolError("failed to get load balancer %s: %v", lbID, err), nil
		}

		preview := fmt.Sprintf("Will DELETE load balancer '%s' (%s), VIP: %s, status: %s",
			lb.Name, lb.ID, lb.VipAddress, lb.ProvisioningStatus)
		if cascade {
			preview += " and ALL associated listeners, pools, members, and health monitors"
		}
		if result := shared.RequireConfirmation(request, preview); result != nil {
			return result, nil
		}

		err = loadbalancers.Delete(ctx, client, lbID, loadbalancers.DeleteOpts{Cascade: cascade}).ExtractErr()
		if err != nil {
			return shared.ToolError("failed to delete load balancer %s: %v", lbID, err), nil
		}

		return shared.ToolResult("Successfully deleted load balancer " + lbID), nil
	}
}
