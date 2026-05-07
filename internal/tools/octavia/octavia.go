// Package octavia provides MCP tools for OpenStack Load Balancer (Octavia) operations.
package octavia

import (
	"context"
	"encoding/json"

	"github.com/gophercloud/gophercloud/v2/openstack/loadbalancer/v2/listeners"
	"github.com/gophercloud/gophercloud/v2/openstack/loadbalancer/v2/loadbalancers"
	"github.com/gophercloud/gophercloud/v2/openstack/loadbalancer/v2/pools"
	"github.com/gophercloud/gophercloud/v2/pagination"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/notque/openstack-mcp-server/internal/auth"
	"github.com/notque/openstack-mcp-server/internal/tools/shared"
)

// Register adds all Octavia tools to the MCP server.
func Register(s *mcpserver.MCPServer, provider *auth.Provider) {
	s.AddTool(listLoadbalancersTool, listLoadbalancersHandler(provider))
	s.AddTool(getLoadbalancerTool, getLoadbalancerHandler(provider))
	s.AddTool(listListenersTool, listListenersHandler(provider))
	s.AddTool(listPoolsTool, listPoolsHandler(provider))
}

// --- Load Balancers ---

var listLoadbalancersTool = mcp.NewTool("octavia_list_loadbalancers",
	mcp.WithDescription("List load balancers in the current project. Returns ID, name, status, VIP address, and provider."),
	mcp.WithString("name", mcp.Description("Filter by load balancer name")),
	mcp.WithString("provisioning_status", mcp.Description("Filter by provisioning status (ACTIVE, PENDING_CREATE, ERROR)")),
	mcp.WithString("vip_address", mcp.Description("Filter by virtual IP address")),
)

var getLoadbalancerTool = mcp.NewTool("octavia_get_loadbalancer",
	mcp.WithDescription("Get detailed information about a specific load balancer."),
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

		var result []map[string]any
		err = loadbalancers.List(client, opts).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
			lbs, err := loadbalancers.ExtractLoadBalancers(page)
			if err != nil {
				return false, err
			}
			for _, lb := range lbs {
				result = append(result, map[string]any{
					"id":                  lb.ID,
					"name":                lb.Name,
					"provisioning_status": lb.ProvisioningStatus,
					"operating_status":    lb.OperatingStatus,
					"vip_address":         lb.VipAddress,
					"vip_subnet_id":       lb.VipSubnetID,
					"provider":            lb.Provider,
					"created_at":          lb.CreatedAt,
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
	mcp.WithString("name", mcp.Description("Filter by listener name")),
	mcp.WithString("protocol", mcp.Description("Filter by protocol (TCP, HTTP, HTTPS, TERMINATED_HTTPS, UDP, SCTP)")),
	mcp.WithString("loadbalancer_id", mcp.Description("Filter by load balancer UUID")),
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
					"id":                  l.ID,
					"name":                l.Name,
					"protocol":            l.Protocol,
					"protocol_port":       l.ProtocolPort,
					"default_pool_id":     l.DefaultPoolID,
					"provisioning_status": l.ProvisioningStatus,
					"loadbalancers":       lbIDs,
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
	mcp.WithString("name", mcp.Description("Filter by pool name")),
	mcp.WithString("protocol", mcp.Description("Filter by protocol (TCP, HTTP, HTTPS, PROXY, UDP, SCTP)")),
	mcp.WithString("loadbalancer_id", mcp.Description("Filter by load balancer UUID")),
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
					"id":                  p.ID,
					"name":                p.Name,
					"protocol":            p.Protocol,
					"lb_method":           p.LBMethod,
					"provisioning_status": p.ProvisioningStatus,
					"operating_status":    p.OperatingStatus,
					"loadbalancers":       lbIDs,
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
