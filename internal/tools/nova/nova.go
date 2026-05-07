// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

// Package nova provides MCP tools for OpenStack Compute (Nova) operations.
package nova

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/flavors"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/servers"
	"github.com/gophercloud/gophercloud/v2/pagination"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/notque/openstack-mcp-server/internal/auth"
	"github.com/notque/openstack-mcp-server/internal/tools/shared"
)

// Register adds all Nova tools to the MCP server.
// When readOnly is true, mutating tools (server actions) are not registered.
func Register(s *mcpserver.MCPServer, provider *auth.Provider, readOnly bool) {
	s.AddTool(listServersTool, listServersHandler(provider))
	s.AddTool(getServerTool, getServerHandler(provider))
	s.AddTool(listFlavorsTool, listFlavorsHandler(provider))
	if !readOnly {
		s.AddTool(serverActionTool, serverActionHandler(provider))
	}
}

// --- Tool Definitions ---

var listServersTool = mcp.NewTool("nova_list_servers",
	mcp.WithDescription("List compute instances (servers) in the current project. Returns server ID, name, status, addresses, and flavor."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithString("status", mcp.Description("Filter by server status (ACTIVE, SHUTOFF, ERROR, BUILD, etc.)")),
	mcp.WithString("name", mcp.Description("Filter by server name (regex supported)")),
	mcp.WithNumber("limit", mcp.Description("Maximum number of servers to return (default: 100)")),
)

var getServerTool = mcp.NewTool("nova_get_server",
	mcp.WithDescription("Get detailed information about a specific compute instance by ID."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithString("server_id", mcp.Required(), mcp.Description("The UUID of the server to retrieve")),
)

var listFlavorsTool = mcp.NewTool("nova_list_flavors",
	mcp.WithDescription("List available compute flavors (instance types) with their specs: vCPUs, RAM, disk."),
	mcp.WithReadOnlyHintAnnotation(true),
)

var serverActionTool = mcp.NewTool("nova_server_action",
	mcp.WithDescription("Perform an action on a compute instance: start, stop, reboot, pause, unpause, suspend, resume."),
	mcp.WithDestructiveHintAnnotation(true),
	mcp.WithString("server_id", mcp.Required(), mcp.Description("The UUID of the server")),
	mcp.WithString("action", mcp.Required(), mcp.Description("Action to perform: start, stop, reboot, pause, unpause, suspend, resume")),
	mcp.WithString("reboot_type", mcp.Description("Reboot type: SOFT or HARD (default: SOFT). Only used with 'reboot' action.")),
)

// --- Handlers ---

func listServersHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.ComputeClient()
		if err != nil {
			return shared.ToolError("failed to get compute client: %v", err), nil
		}

		opts := servers.ListOpts{
			Status: shared.StringParam(request, "status"),
			Name:   shared.StringParam(request, "name"),
		}

		var maxResults int
		if limit := shared.NumberParam(request, "limit"); limit > 0 {
			opts.Limit = int(limit)
			maxResults = int(limit)
		} else {
			opts.Limit = 100
			maxResults = 100
		}

		var result []map[string]any
		err = servers.List(client, opts).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
			srvs, err := servers.ExtractServers(page)
			if err != nil {
				return false, err
			}
			for _, s := range srvs {
				result = append(result, map[string]any{
					"id":        s.ID,
					"name":      s.Name,
					"status":    s.Status,
					"addresses": s.Addresses,
					"flavor":    s.Flavor,
					"created":   s.Created,
					"updated":   s.Updated,
					"host_id":   s.HostID,
				})
			}
			// Stop paginating once we have enough results
			return len(result) < maxResults, nil
		})
		if err != nil {
			return shared.ToolError("failed to list servers: %v", err), nil
		}

		out, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

func getServerHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.ComputeClient()
		if err != nil {
			return shared.ToolError("failed to get compute client: %v", err), nil
		}

		serverID := shared.StringParam(request, "server_id")
		if serverID == "" {
			return shared.ToolError("server_id is required"), nil
		}

		srv, err := servers.Get(ctx, client, serverID).Extract()
		if err != nil {
			return shared.ToolError("failed to get server %s: %v", serverID, err), nil
		}

		out, err := json.MarshalIndent(srv, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

func listFlavorsHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.ComputeClient()
		if err != nil {
			return shared.ToolError("failed to get compute client: %v", err), nil
		}

		var result []map[string]any
		err = flavors.ListDetail(client, nil).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
			flvs, err := flavors.ExtractFlavors(page)
			if err != nil {
				return false, err
			}
			for _, f := range flvs {
				result = append(result, map[string]any{
					"id":    f.ID,
					"name":  f.Name,
					"vcpus": f.VCPUs,
					"ram":   f.RAM,
					"disk":  f.Disk,
				})
			}
			return true, nil
		})
		if err != nil {
			return shared.ToolError("failed to list flavors: %v", err), nil
		}

		out, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

func serverActionHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.ComputeClient()
		if err != nil {
			return shared.ToolError("failed to get compute client: %v", err), nil
		}

		serverID := shared.StringParam(request, "server_id")
		action := shared.StringParam(request, "action")

		if serverID == "" || action == "" {
			return shared.ToolError("server_id and action are required"), nil
		}

		switch action {
		case "start":
			err = servers.Start(ctx, client, serverID).ExtractErr()
		case "stop":
			err = servers.Stop(ctx, client, serverID).ExtractErr()
		case "reboot":
			rebootType := servers.SoftReboot
			if shared.StringParam(request, "reboot_type") == "HARD" {
				rebootType = servers.HardReboot
			}
			err = servers.Reboot(ctx, client, serverID, servers.RebootOpts{Type: rebootType}).ExtractErr()
		case "pause":
			err = servers.Pause(ctx, client, serverID).ExtractErr()
		case "unpause":
			err = servers.Unpause(ctx, client, serverID).ExtractErr()
		case "suspend":
			err = servers.Suspend(ctx, client, serverID).ExtractErr()
		case "resume":
			err = servers.Resume(ctx, client, serverID).ExtractErr()
		default:
			return shared.ToolError("unsupported action: %s (valid: start, stop, reboot, pause, unpause, suspend, resume)", action), nil
		}

		if err != nil {
			return shared.ToolError("failed to %s server %s: %v", action, serverID, err), nil
		}

		return shared.ToolResult(fmt.Sprintf("Successfully performed '%s' on server %s", action, serverID)), nil
	}
}
