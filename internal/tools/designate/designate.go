// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

// Package designate provides MCP tools for OpenStack DNS (Designate) operations.
package designate

import (
	"context"
	"encoding/json"

	"github.com/gophercloud/gophercloud/v2/openstack/dns/v2/recordsets"
	"github.com/gophercloud/gophercloud/v2/openstack/dns/v2/zones"
	"github.com/gophercloud/gophercloud/v2/pagination"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/notque/openstack-mcp-server/internal/auth"
	"github.com/notque/openstack-mcp-server/internal/tools/shared"
)

// Register adds all Designate tools to the MCP server.
func Register(s *mcpserver.MCPServer, provider *auth.Provider) {
	s.AddTool(listZonesTool, listZonesHandler(provider))
	s.AddTool(getZoneTool, getZoneHandler(provider))
	s.AddTool(listRecordsetsTool, listRecordsetsHandler(provider))
}

var listZonesTool = mcp.NewTool("designate_list_zones",
	mcp.WithDescription("List DNS zones in the current project. Returns zone ID, name, email, TTL, status, type, serial, and created_at."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithString("name", mcp.Description("Filter by zone name")),
	mcp.WithString("status", mcp.Description("Filter by zone status (ACTIVE, PENDING, ERROR)")),
	mcp.WithString("type", mcp.Description("Filter by zone type (PRIMARY, SECONDARY)")),
)

var getZoneTool = mcp.NewTool("designate_get_zone",
	mcp.WithDescription("Get detailed information about a specific DNS zone."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithString("zone_id", mcp.Required(), mcp.Description("The UUID of the zone to retrieve")),
)

var listRecordsetsTool = mcp.NewTool("designate_list_recordsets",
	mcp.WithDescription("List DNS recordsets in a zone. Returns recordset ID, name, type, records, TTL, and status."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithString("zone_id", mcp.Required(), mcp.Description("The UUID of the zone to list recordsets for")),
	mcp.WithString("name", mcp.Description("Filter by recordset name")),
	mcp.WithString("type", mcp.Description("Filter by recordset type (A, AAAA, CNAME, MX, TXT, etc.)")),
	mcp.WithString("status", mcp.Description("Filter by recordset status (ACTIVE, PENDING, ERROR)")),
	mcp.WithString("data", mcp.Description("Filter by record data/value (e.g., an IP address or CNAME target)")),
)

func listZonesHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.DNSClient()
		if err != nil {
			return shared.ToolError("failed to get DNS client: %v", err), nil
		}

		opts := zones.ListOpts{
			Name:   shared.StringParam(request, "name"),
			Status: shared.StringParam(request, "status"),
			Type:   shared.StringParam(request, "type"),
		}

		var result []map[string]any
		err = zones.List(client, opts).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
			zoneList, err := zones.ExtractZones(page)
			if err != nil {
				return false, err
			}
			for _, z := range zoneList {
				result = append(result, map[string]any{
					"id":         z.ID,
					"name":       z.Name,
					"email":      z.Email,
					"ttl":        z.TTL,
					"status":     z.Status,
					"type":       z.Type,
					"serial":     z.Serial,
					"created_at": z.CreatedAt,
				})
			}
			return true, nil
		})
		if err != nil {
			return shared.ToolError("failed to list zones: %v", err), nil
		}

		out, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

func getZoneHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.DNSClient()
		if err != nil {
			return shared.ToolError("failed to get DNS client: %v", err), nil
		}

		zoneID := shared.StringParam(request, "zone_id")
		if zoneID == "" {
			return shared.ToolError("zone_id is required"), nil
		}

		zone, err := zones.Get(ctx, client, zoneID).Extract()
		if err != nil {
			return shared.ToolError("failed to get zone %s: %v", zoneID, err), nil
		}

		out, err := json.MarshalIndent(zone, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

func listRecordsetsHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.DNSClient()
		if err != nil {
			return shared.ToolError("failed to get DNS client: %v", err), nil
		}

		zoneID := shared.StringParam(request, "zone_id")
		if zoneID == "" {
			return shared.ToolError("zone_id is required"), nil
		}

		opts := recordsets.ListOpts{
			Name:   shared.StringParam(request, "name"),
			Type:   shared.StringParam(request, "type"),
			Status: shared.StringParam(request, "status"),
			Data:   shared.StringParam(request, "data"),
		}

		var result []map[string]any
		err = recordsets.ListByZone(client, zoneID, opts).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
			rsList, err := recordsets.ExtractRecordSets(page)
			if err != nil {
				return false, err
			}
			for _, rs := range rsList {
				result = append(result, map[string]any{
					"id":      rs.ID,
					"name":    rs.Name,
					"type":    rs.Type,
					"records": rs.Records,
					"ttl":     rs.TTL,
					"status":  rs.Status,
				})
			}
			return true, nil
		})
		if err != nil {
			return shared.ToolError("failed to list recordsets: %v", err), nil
		}

		out, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}
