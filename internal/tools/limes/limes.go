// Package limes provides MCP tools for SAP CC Limes (Quota/Usage) service.
// Limes tracks resource quotas and usage across OpenStack projects.
package limes

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/notque/openstack-mcp-server/internal/auth"
	"github.com/notque/openstack-mcp-server/internal/tools/shared"
)

// Register adds all Limes tools to the MCP server.
func Register(s *mcpserver.MCPServer, provider *auth.Provider) {
	s.AddTool(getProjectQuotaTool, getProjectQuotaHandler(provider))
	s.AddTool(getDomainQuotaTool, getDomainQuotaHandler(provider))
	s.AddTool(getClusterQuotaTool, getClusterQuotaHandler(provider))
}

var getProjectQuotaTool = mcp.NewTool("limes_get_project_quota",
	mcp.WithDescription("Get quota and usage report for a specific project. Shows all services (compute, network, storage, etc.) with their quota limits, current usage, and physical usage."),
	mcp.WithString("domain_id", mcp.Required(), mcp.Description("The domain ID containing the project")),
	mcp.WithString("project_id", mcp.Required(), mcp.Description("The project ID to get quota for")),
	mcp.WithString("service", mcp.Description("Filter by service type (e.g., 'compute', 'network', 'object-store')")),
	mcp.WithString("resource", mcp.Description("Filter by resource name (e.g., 'cores', 'ram', 'instances')")),
)

var getDomainQuotaTool = mcp.NewTool("limes_get_domain_quota",
	mcp.WithDescription("Get aggregated quota and usage report for all projects in a domain."),
	mcp.WithString("domain_id", mcp.Required(), mcp.Description("The domain ID to get quota for")),
	mcp.WithString("service", mcp.Description("Filter by service type")),
)

var getClusterQuotaTool = mcp.NewTool("limes_get_cluster_quota",
	mcp.WithDescription("Get cluster-wide capacity and usage information. Shows total capacity, used capacity, and remaining capacity per service."),
	mcp.WithString("service", mcp.Description("Filter by service type")),
)

func getProjectQuotaHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.LimesClient()
		if err != nil {
			return shared.ToolError("failed to get limes client: %v", err), nil
		}

		domainID := shared.StringParam(request, "domain_id")
		projectID := shared.StringParam(request, "project_id")
		if domainID == "" || projectID == "" {
			return shared.ToolError("domain_id and project_id are required"), nil
		}

		url := client.Endpoint + "domains/" + domainID + "/projects/" + projectID
		if svc := shared.StringParam(request, "service"); svc != "" {
			url += "?service=" + svc
		}

		var body any
		//nolint:bodyclose
		_, err = client.Get(ctx, url, &body, &gophercloud.RequestOpts{
			OkCodes: []int{http.StatusOK},
		})
		if err != nil {
			return shared.ToolError("failed to get project quota: %v", err), nil
		}

		out, err := json.MarshalIndent(body, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

func getDomainQuotaHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.LimesClient()
		if err != nil {
			return shared.ToolError("failed to get limes client: %v", err), nil
		}

		domainID := shared.StringParam(request, "domain_id")
		if domainID == "" {
			return shared.ToolError("domain_id is required"), nil
		}

		url := client.Endpoint + "domains/" + domainID
		if svc := shared.StringParam(request, "service"); svc != "" {
			url += "?service=" + svc
		}

		var body any
		//nolint:bodyclose
		_, err = client.Get(ctx, url, &body, &gophercloud.RequestOpts{
			OkCodes: []int{http.StatusOK},
		})
		if err != nil {
			return shared.ToolError("failed to get domain quota: %v", err), nil
		}

		out, err := json.MarshalIndent(body, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

func getClusterQuotaHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.LimesClient()
		if err != nil {
			return shared.ToolError("failed to get limes client: %v", err), nil
		}

		url := client.Endpoint + "clusters/current"
		if svc := shared.StringParam(request, "service"); svc != "" {
			url += "?service=" + svc
		}

		var body any
		//nolint:bodyclose
		_, err = client.Get(ctx, url, &body, &gophercloud.RequestOpts{
			OkCodes: []int{http.StatusOK},
		})
		if err != nil {
			return shared.ToolError("failed to get cluster quota: %v", err), nil
		}

		out, err := json.MarshalIndent(body, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}
