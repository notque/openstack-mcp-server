// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

// Package maia provides MCP tools for SAP CC Maia (Prometheus-as-a-Service).
// Maia offers multi-tenant Prometheus-compatible metrics with OpenStack auth.
package maia

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/notque/openstack-mcp-server/internal/auth"
	"github.com/notque/openstack-mcp-server/internal/tools/shared"
)

// Register adds all Maia tools to the MCP server.
func Register(s *mcpserver.MCPServer, provider *auth.Provider) {
	s.AddTool(queryTool, queryHandler(provider))
	s.AddTool(labelValuesTool, labelValuesHandler(provider))
	s.AddTool(metricNamesTool, metricNamesHandler(provider))
}

var queryTool = mcp.NewTool("maia_query",
	mcp.WithDescription("Execute a PromQL query against Maia (multi-tenant Prometheus). Returns instant query results scoped to the current project."),
	mcp.WithString("query", mcp.Required(), mcp.Description("PromQL expression to evaluate (e.g., 'up', 'node_cpu_seconds_total{mode=\"idle\"}')")),
	mcp.WithString("time", mcp.Description("Evaluation timestamp (RFC3339 or Unix). Defaults to current time.")),
)

var labelValuesTool = mcp.NewTool("maia_label_values",
	mcp.WithDescription("Get all values for a specific Prometheus label in Maia. Useful for discovering available metrics and dimensions."),
	mcp.WithString("label", mcp.Required(), mcp.Description("The label name to get values for (e.g., '__name__' for metric names, 'instance', 'job')")),
)

var metricNamesTool = mcp.NewTool("maia_metric_names",
	mcp.WithDescription("List all available metric names in Maia for the current project scope."),
)

func queryHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.MaiaClient()
		if err != nil {
			return shared.ToolError("failed to get maia client: %v", err), nil
		}

		query := shared.StringParam(request, "query")
		if query == "" {
			return shared.ToolError("query is required"), nil
		}

		apiURL := client.Endpoint + "api/v1/query?query=" + url.QueryEscape(query)
		if t := shared.StringParam(request, "time"); t != "" {
			apiURL += "&time=" + url.QueryEscape(t)
		}

		var body any
		//nolint:bodyclose
		_, err = client.Get(ctx, apiURL, &body, &gophercloud.RequestOpts{
			OkCodes: []int{http.StatusOK},
		})
		if err != nil {
			return shared.ToolError("failed to query maia: %v", err), nil
		}

		out, err := json.MarshalIndent(body, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

func labelValuesHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.MaiaClient()
		if err != nil {
			return shared.ToolError("failed to get maia client: %v", err), nil
		}

		label := shared.StringParam(request, "label")
		if label == "" {
			return shared.ToolError("label is required"), nil
		}

		apiURL := client.Endpoint + "api/v1/label/" + url.PathEscape(label) + "/values"

		var body any
		//nolint:bodyclose
		_, err = client.Get(ctx, apiURL, &body, &gophercloud.RequestOpts{
			OkCodes: []int{http.StatusOK},
		})
		if err != nil {
			return shared.ToolError("failed to get label values for %s: %v", label, err), nil
		}

		out, err := json.MarshalIndent(body, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

func metricNamesHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.MaiaClient()
		if err != nil {
			return shared.ToolError("failed to get maia client: %v", err), nil
		}

		apiURL := client.Endpoint + "api/v1/label/__name__/values"

		var body any
		//nolint:bodyclose
		_, err = client.Get(ctx, apiURL, &body, &gophercloud.RequestOpts{
			OkCodes: []int{http.StatusOK},
		})
		if err != nil {
			return shared.ToolError("failed to get metric names: %v", err), nil
		}

		out, err := json.MarshalIndent(body, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}
