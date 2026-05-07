// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

// Package cronus provides MCP tools for SAP CC Cronus (Email) service operations.
// Cronus handles email sending capabilities within SAP Converged Cloud.
package cronus

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

// Register adds all Cronus tools to the MCP server.
func Register(s *mcpserver.MCPServer, provider *auth.Provider) {
	s.AddTool(getUsageTool, getUsageHandler(provider))
	s.AddTool(listTemplatesTool, listTemplatesHandler(provider))
}

var getUsageTool = mcp.NewTool("cronus_get_usage",
	mcp.WithDescription("Get email sending usage and status for the current project from Cronus."),
	mcp.WithReadOnlyHintAnnotation(true),
)

var listTemplatesTool = mcp.NewTool("cronus_list_templates",
	mcp.WithDescription("List available email templates in Cronus for the current project."),
	mcp.WithReadOnlyHintAnnotation(true),
)

func getUsageHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.CronusClient()
		if err != nil {
			return shared.ToolError("failed to get cronus client: %v", err), nil
		}

		reqURL := client.Endpoint + "v1/usage"

		var body any
		//nolint:bodyclose
		_, err = client.Get(ctx, reqURL, &body, &gophercloud.RequestOpts{
			OkCodes: []int{http.StatusOK},
		})
		if err != nil {
			return shared.ToolError("failed to get cronus usage: %v", err), nil
		}

		out, err := json.MarshalIndent(body, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

func listTemplatesHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.CronusClient()
		if err != nil {
			return shared.ToolError("failed to get cronus client: %v", err), nil
		}

		reqURL := client.Endpoint + "v1/templates"

		var body any
		//nolint:bodyclose
		_, err = client.Get(ctx, reqURL, &body, &gophercloud.RequestOpts{
			OkCodes: []int{http.StatusOK},
		})
		if err != nil {
			return shared.ToolError("failed to list cronus templates: %v", err), nil
		}

		out, err := json.MarshalIndent(body, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}
