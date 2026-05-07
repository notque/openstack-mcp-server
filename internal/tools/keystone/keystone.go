// Package keystone provides MCP tools for OpenStack Identity (Keystone) operations.
package keystone

import (
	"context"
	"encoding/json"

	"github.com/gophercloud/gophercloud/v2/openstack/identity/v3/projects"
	"github.com/gophercloud/gophercloud/v2/openstack/identity/v3/tokens"
	"github.com/gophercloud/gophercloud/v2/pagination"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/notque/openstack-mcp-server/internal/auth"
	"github.com/notque/openstack-mcp-server/internal/tools/shared"
)

// Register adds all Keystone tools to the MCP server.
func Register(s *mcpserver.MCPServer, provider *auth.Provider) {
	s.AddTool(listProjectsTool, listProjectsHandler(provider))
	s.AddTool(tokenInfoTool, tokenInfoHandler(provider))
}

var listProjectsTool = mcp.NewTool("keystone_list_projects",
	mcp.WithDescription("List projects (tenants) accessible to the current user. Returns project ID, name, domain, and enabled status."),
	mcp.WithString("domain_id", mcp.Description("Filter by domain ID")),
	mcp.WithString("name", mcp.Description("Filter by project name")),
)

var tokenInfoTool = mcp.NewTool("keystone_token_info",
	mcp.WithDescription("Get information about the current authentication token: user, project, domain, roles, and service catalog."),
)

func listProjectsHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.IdentityClient()
		if err != nil {
			return shared.ToolError("failed to get identity client: %v", err), nil
		}

		opts := projects.ListOpts{
			DomainID: shared.StringParam(request, "domain_id"),
			Name:     shared.StringParam(request, "name"),
		}

		var result []map[string]any
		err = projects.List(client, opts).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
			ps, err := projects.ExtractProjects(page)
			if err != nil {
				return false, err
			}
			for _, p := range ps {
				result = append(result, map[string]any{
					"id":        p.ID,
					"name":      p.Name,
					"domain_id": p.DomainID,
					"enabled":   p.Enabled,
				})
			}
			return true, nil
		})
		if err != nil {
			return shared.ToolError("failed to list projects: %v", err), nil
		}

		out, _ := json.MarshalIndent(result, "", "  ")
		return shared.ToolResult(string(out)), nil
	}
}

func tokenInfoHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.IdentityClient()
		if err != nil {
			return shared.ToolError("failed to get identity client: %v", err), nil
		}

		result := tokens.Get(ctx, client, provider.Token())

		token, err := result.Extract()
		if err != nil {
			return shared.ToolError("failed to get token info: %v", err), nil
		}

		info := map[string]any{
			"expires_at": token.ExpiresAt,
		}

		if user, err := result.ExtractUser(); err == nil {
			info["user"] = user
		}
		if project, err := result.ExtractProject(); err == nil {
			info["project"] = project
		}
		if domain, err := result.ExtractDomain(); err == nil {
			info["domain"] = domain
		}
		if roles, err := result.ExtractRoles(); err == nil {
			info["roles"] = roles
		}
		if catalog, err := result.ExtractServiceCatalog(); err == nil {
			info["service_catalog"] = catalog
		}

		out, _ := json.MarshalIndent(info, "", "  ")
		return shared.ToolResult(string(out)), nil
	}
}
