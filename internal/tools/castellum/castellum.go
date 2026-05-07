// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

// Package castellum provides MCP tools for SAP CC Castellum (Autoscaling) operations.
// Castellum provides automatic resource scaling based on configured thresholds.
package castellum

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/notque/openstack-mcp-server/internal/auth"
	"github.com/notque/openstack-mcp-server/internal/tools/shared"
)

// Register adds all Castellum tools to the MCP server.
func Register(s *mcpserver.MCPServer, provider *auth.Provider) {
	s.AddTool(getProjectResourcesTool, getProjectResourcesHandler(provider))
	s.AddTool(listPendingOperationsTool, listPendingOperationsHandler(provider))
	s.AddTool(listRecentlyFailedOperationsTool, listRecentlyFailedOperationsHandler(provider))
}

var getProjectResourcesTool = mcp.NewTool("castellum_get_project_resources",
	mcp.WithDescription("Get autoscaling configuration and resource status for a project from Castellum."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithString("project_id", mcp.Required(), mcp.Description("The UUID of the project to retrieve autoscaling configuration for")),
)

var listPendingOperationsTool = mcp.NewTool("castellum_list_pending_operations",
	mcp.WithDescription("List pending resize operations in Castellum. These are operations that have been scheduled but not yet completed."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithString("project_id", mcp.Description("Filter by project UUID")),
	mcp.WithString("asset_type", mcp.Description("Filter by asset type (e.g., 'project-quota:compute:cores')")),
)

var listRecentlyFailedOperationsTool = mcp.NewTool("castellum_list_recently_failed_operations",
	mcp.WithDescription("List recently failed resize operations in Castellum. Useful for diagnosing autoscaling issues."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithString("project_id", mcp.Description("Filter by project UUID")),
	mcp.WithString("asset_type", mcp.Description("Filter by asset type (e.g., 'project-quota:compute:cores')")),
	mcp.WithString("max_age", mcp.Description("Time window for results (e.g., '12h', '7d'). Default: '1d'")),
)

func getProjectResourcesHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.CastellumClient()
		if err != nil {
			return shared.ToolError("failed to get castellum client: %v", err), nil
		}

		projectID := shared.StringParam(request, "project_id")
		if projectID == "" {
			return shared.ToolError("project_id is required"), nil
		}
		if errResult := shared.ValidateUUID(projectID, "project_id"); errResult != nil {
			return errResult, nil
		}

		reqURL := client.Endpoint + "v1/projects/" + projectID

		var body any
		//nolint:bodyclose
		_, err = client.Get(ctx, reqURL, &body, &gophercloud.RequestOpts{
			OkCodes: []int{http.StatusOK},
		})
		if err != nil {
			return shared.ToolError("failed to get castellum project resources for %s: %v", projectID, err), nil
		}

		out, err := json.MarshalIndent(body, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

func listPendingOperationsHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.CastellumClient()
		if err != nil {
			return shared.ToolError("failed to get castellum client: %v", err), nil
		}

		query, err := buildOperationsQuery(request)
		if err != nil {
			return shared.ToolError("%v", err), nil
		}

		reqURL := client.Endpoint + "v1/operations/pending" + query

		var body any
		//nolint:bodyclose
		_, err = client.Get(ctx, reqURL, &body, &gophercloud.RequestOpts{
			OkCodes: []int{http.StatusOK},
		})
		if err != nil {
			return shared.ToolError("failed to list pending operations: %v", err), nil
		}

		out, err := json.MarshalIndent(body, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

func listRecentlyFailedOperationsHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.CastellumClient()
		if err != nil {
			return shared.ToolError("failed to get castellum client: %v", err), nil
		}

		query, err := buildOperationsQuery(request)
		if err != nil {
			return shared.ToolError("%v", err), nil
		}

		reqURL := client.Endpoint + "v1/operations/recently-failed" + query
		if maxAge := shared.StringParam(request, "max_age"); maxAge != "" {
			if query == "" {
				reqURL += "?max-age=" + url.QueryEscape(maxAge)
			} else {
				reqURL += "&max-age=" + url.QueryEscape(maxAge)
			}
		}

		var body any
		//nolint:bodyclose
		_, err = client.Get(ctx, reqURL, &body, &gophercloud.RequestOpts{
			OkCodes: []int{http.StatusOK},
		})
		if err != nil {
			return shared.ToolError("failed to list recently failed operations: %v", err), nil
		}

		out, err := json.MarshalIndent(body, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

// buildOperationsQuery constructs query parameters for operations endpoints.
// Returns an error if project_id is present but not a valid UUID.
func buildOperationsQuery(request mcp.CallToolRequest) (string, error) {
	params := url.Values{}

	if projectID := shared.StringParam(request, "project_id"); projectID != "" {
		if errResult := shared.ValidateUUID(projectID, "project_id"); errResult != nil {
			return "", errors.New("project_id must be a valid UUID")
		}
		params.Set("project", projectID)
	}
	if assetType := shared.StringParam(request, "asset_type"); assetType != "" {
		params.Set("asset-type", assetType)
	}

	if len(params) == 0 {
		return "", nil
	}
	return "?" + params.Encode(), nil
}
