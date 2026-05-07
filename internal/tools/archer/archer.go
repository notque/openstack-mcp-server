// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

// Package archer provides MCP tools for SAP CC Archer (Endpoint Service).
// Archer enables private network connectivity between OpenStack networks
// through service endpoints.
package archer

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

// Register adds all Archer tools to the MCP server.
func Register(s *mcpserver.MCPServer, provider *auth.Provider) {
	s.AddTool(listServicesTool, listServicesHandler(provider))
	s.AddTool(listEndpointsTool, listEndpointsHandler(provider))
	s.AddTool(getServiceTool, getServiceHandler(provider))
	s.AddTool(getEndpointTool, getEndpointHandler(provider))
}

var listServicesTool = mcp.NewTool("archer_list_services",
	mcp.WithDescription("List Archer services available for endpoint creation. Services are private/public network resources accessible through endpoints."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithString("status", mcp.Description("Filter by service status")),
)

var listEndpointsTool = mcp.NewTool("archer_list_endpoints",
	mcp.WithDescription("List Archer endpoints in the current project. Endpoints provide local IP access to remote services."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithString("service_id", mcp.Description("Filter by the service this endpoint connects to")),
	mcp.WithString("status", mcp.Description("Filter by endpoint status")),
)

var getServiceTool = mcp.NewTool("archer_get_service",
	mcp.WithDescription("Get details of a specific Archer service."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithString("service_id", mcp.Required(), mcp.Description("The UUID of the service")),
)

var getEndpointTool = mcp.NewTool("archer_get_endpoint",
	mcp.WithDescription("Get details of a specific Archer endpoint."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithString("endpoint_id", mcp.Required(), mcp.Description("The UUID of the endpoint")),
)

func listServicesHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.ArcherClient()
		if err != nil {
			return shared.ToolError("failed to get archer client: %v", err), nil
		}

		url := client.Endpoint + "service"
		if status := shared.StringParam(request, "status"); status != "" {
			url += "?status=" + status
		}

		var body any
		//nolint:bodyclose
		_, err = client.Get(ctx, url, &body, &gophercloud.RequestOpts{
			OkCodes: []int{http.StatusOK},
		})
		if err != nil {
			return shared.ToolError("failed to list archer services: %v", err), nil
		}

		out, err := json.MarshalIndent(body, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

func listEndpointsHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.ArcherClient()
		if err != nil {
			return shared.ToolError("failed to get archer client: %v", err), nil
		}

		url := client.Endpoint + "endpoint"
		sep := "?"
		if svcID := shared.StringParam(request, "service_id"); svcID != "" {
			url += sep + "service_id=" + svcID
			sep = "&"
		}
		if status := shared.StringParam(request, "status"); status != "" {
			url += sep + "status=" + status
		}

		var body any
		//nolint:bodyclose
		_, err = client.Get(ctx, url, &body, &gophercloud.RequestOpts{
			OkCodes: []int{http.StatusOK},
		})
		if err != nil {
			return shared.ToolError("failed to list archer endpoints: %v", err), nil
		}

		out, err := json.MarshalIndent(body, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

func getServiceHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.ArcherClient()
		if err != nil {
			return shared.ToolError("failed to get archer client: %v", err), nil
		}

		serviceID := shared.StringParam(request, "service_id")
		if serviceID == "" {
			return shared.ToolError("service_id is required"), nil
		}

		url := client.Endpoint + "service/" + serviceID

		var body any
		//nolint:bodyclose
		_, err = client.Get(ctx, url, &body, &gophercloud.RequestOpts{
			OkCodes: []int{http.StatusOK},
		})
		if err != nil {
			return shared.ToolError("failed to get archer service %s: %v", serviceID, err), nil
		}

		out, err := json.MarshalIndent(body, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

func getEndpointHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.ArcherClient()
		if err != nil {
			return shared.ToolError("failed to get archer client: %v", err), nil
		}

		endpointID := shared.StringParam(request, "endpoint_id")
		if endpointID == "" {
			return shared.ToolError("endpoint_id is required"), nil
		}

		url := client.Endpoint + "endpoint/" + endpointID

		var body any
		//nolint:bodyclose
		_, err = client.Get(ctx, url, &body, &gophercloud.RequestOpts{
			OkCodes: []int{http.StatusOK},
		})
		if err != nil {
			return shared.ToolError("failed to get archer endpoint %s: %v", endpointID, err), nil
		}

		out, err := json.MarshalIndent(body, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}
