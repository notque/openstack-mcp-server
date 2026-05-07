// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

// Package designate provides MCP tools for OpenStack DNS (Designate) operations.
package designate

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/gophercloud/gophercloud/v2/openstack/dns/v2/recordsets"
	transferaccept "github.com/gophercloud/gophercloud/v2/openstack/dns/v2/transfer/accept"
	transferrequest "github.com/gophercloud/gophercloud/v2/openstack/dns/v2/transfer/request"
	"github.com/gophercloud/gophercloud/v2/openstack/dns/v2/zones"
	"github.com/gophercloud/gophercloud/v2/pagination"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/notque/openstack-mcp-server/internal/auth"
	"github.com/notque/openstack-mcp-server/internal/tools/shared"
)

// Register adds all Designate tools to the MCP server.
// When readOnly is true, mutating tools are not registered.
func Register(s *mcpserver.MCPServer, provider *auth.Provider, readOnly, _ bool) {
	s.AddTool(listZonesTool, listZonesHandler(provider))
	s.AddTool(getZoneTool, getZoneHandler(provider))
	s.AddTool(listRecordsetsTool, listRecordsetsHandler(provider))
	s.AddTool(listZoneTransferRequestsTool, listZoneTransferRequestsHandler(provider))
	s.AddTool(listZoneTransferAcceptsTool, listZoneTransferAcceptsHandler(provider))

	if !readOnly {
		s.AddTool(createRecordsetTool, createRecordsetHandler(provider))
		s.AddTool(deleteRecordsetTool, deleteRecordsetHandler(provider))
	}
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
		if errResult := shared.ValidateUUID(zoneID, "zone_id"); errResult != nil {
			return errResult, nil
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
		if errResult := shared.ValidateUUID(zoneID, "zone_id"); errResult != nil {
			return errResult, nil
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

// --- Zone Transfer Requests ---

var listZoneTransferRequestsTool = mcp.NewTool("designate_list_zone_transfer_requests",
	mcp.WithDescription("List outgoing zone transfer requests. Returns transfer request ID, zone ID, zone name, target project ID, status, key, and created_at."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithString("zone_id", mcp.Description("Filter by zone UUID")),
	mcp.WithString("status", mcp.Description("Filter by transfer request status")),
)

func listZoneTransferRequestsHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.DNSClient()
		if err != nil {
			return shared.ToolError("failed to get DNS client: %v", err), nil
		}

		opts := transferrequest.ListOpts{
			Status: shared.StringParam(request, "status"),
		}
		zoneIDFilter := shared.StringParam(request, "zone_id")
		if zoneIDFilter != "" {
			if errResult := shared.ValidateUUID(zoneIDFilter, "zone_id"); errResult != nil {
				return errResult, nil
			}
		}

		result := make([]map[string]any, 0)
		err = transferrequest.List(client, opts).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
			allRequests, err := transferrequest.ExtractTransferRequests(page)
			if err != nil {
				return false, err
			}
			for _, tr := range allRequests {
				if zoneIDFilter != "" && tr.ZoneID != zoneIDFilter {
					continue
				}
				// SECURITY: Do NOT include tr.Key — it is an out-of-band shared
				// secret required to accept transfers. Exposing it to the LLM
				// violates credential isolation.
				result = append(result, map[string]any{
					"id":                tr.ID,
					"zone_id":           tr.ZoneID,
					"zone_name":         tr.ZoneName,
					"target_project_id": tr.TargetProjectID,
					"status":            tr.Status,
					"created_at":        tr.CreatedAt,
				})
			}
			return true, nil
		})
		if err != nil {
			return shared.ToolError("failed to list zone transfer requests: %v", err), nil
		}

		out, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

// --- Zone Transfer Accepts ---

var listZoneTransferAcceptsTool = mcp.NewTool("designate_list_zone_transfer_accepts",
	mcp.WithDescription("List accepted zone transfers. Returns transfer accept ID, zone ID, zone transfer request ID, project ID, status, and created_at."),
	mcp.WithReadOnlyHintAnnotation(true),
)

func listZoneTransferAcceptsHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.DNSClient()
		if err != nil {
			return shared.ToolError("failed to get DNS client: %v", err), nil
		}

		result := make([]map[string]any, 0)
		err = transferaccept.List(client, nil).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
			allAccepts, err := transferaccept.ExtractTransferAccepts(page)
			if err != nil {
				return false, err
			}
			for _, ta := range allAccepts {
				result = append(result, map[string]any{
					"id":                       ta.ID,
					"zone_id":                  ta.ZoneID,
					"zone_transfer_request_id": ta.ZoneTransferRequestID,
					"project_id":               ta.ProjectID,
					"status":                   ta.Status,
					"created_at":               ta.CreatedAt,
				})
			}
			return true, nil
		})
		if err != nil {
			return shared.ToolError("failed to list zone transfer accepts: %v", err), nil
		}

		out, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

// --- Write tools ---

var createRecordsetTool = mcp.NewTool("designate_create_recordset",
	mcp.WithDescription("Create a DNS recordset in a zone."),
	mcp.WithDestructiveHintAnnotation(true),
	mcp.WithString("zone_id", mcp.Required(), mcp.Description("The UUID of the zone to create the recordset in")),
	mcp.WithString("name", mcp.Required(), mcp.Description("Fully qualified domain name ending with '.' (e.g., 'app.example.com.')")),
	mcp.WithString("type", mcp.Required(), mcp.Description("Record type: A, AAAA, CNAME, MX, TXT, SRV, or NS")),
	mcp.WithString("records", mcp.Required(), mcp.Description("Comma-separated record values (e.g., '192.168.1.1,192.168.1.2')")),
	mcp.WithNumber("ttl", mcp.Description("Time to live in seconds")),
	mcp.WithString("description", mcp.Description("Description of the recordset")),
	mcp.WithBoolean("confirmed", mcp.Description("Set to true to execute. Without this, returns a preview of the action.")),
)

var deleteRecordsetTool = mcp.NewTool("designate_delete_recordset",
	mcp.WithDescription("Delete a DNS recordset from a zone."),
	mcp.WithDestructiveHintAnnotation(true),
	mcp.WithString("zone_id", mcp.Required(), mcp.Description("The UUID of the zone containing the recordset")),
	mcp.WithString("recordset_id", mcp.Required(), mcp.Description("The UUID of the recordset to delete")),
	mcp.WithBoolean("confirmed", mcp.Description("Set to true to execute. Without this, returns a preview of the action.")),
)

func createRecordsetHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.DNSClient()
		if err != nil {
			return shared.ToolError("failed to get DNS client: %v", err), nil
		}

		zoneID := shared.StringParam(request, "zone_id")
		if zoneID == "" {
			return shared.ToolError("zone_id is required"), nil
		}
		if errResult := shared.ValidateUUID(zoneID, "zone_id"); errResult != nil {
			return errResult, nil
		}

		name := shared.StringParam(request, "name")
		if name == "" {
			return shared.ToolError("name is required"), nil
		}
		if !strings.HasSuffix(name, ".") {
			return shared.ToolError("DNS name must be a fully qualified domain name ending with '.'"), nil
		}

		recType := shared.StringParam(request, "type")
		if recType == "" {
			return shared.ToolError("type is required"), nil
		}

		recordsStr := shared.StringParam(request, "records")
		if recordsStr == "" {
			return shared.ToolError("records is required"), nil
		}
		records := strings.Split(recordsStr, ",")
		for i := range records {
			records[i] = strings.TrimSpace(records[i])
		}

		if recType == "CNAME" && len(records) > 1 {
			return shared.ToolError("CNAME records must have exactly one value"), nil
		}

		ttl := int(shared.NumberParam(request, "ttl"))
		description := shared.StringParam(request, "description")

		ttlDisplay := "default"
		if ttl > 0 {
			ttlDisplay = strconv.Itoa(ttl)
		}
		preview := fmt.Sprintf("Will CREATE %s record '%s' in zone %s with values: [%s], TTL: %s",
			recType, name, zoneID, strings.Join(records, ", "), ttlDisplay)
		if result := shared.RequireConfirmation(request, preview); result != nil {
			return result, nil
		}

		createOpts := recordsets.CreateOpts{
			Name:        name,
			Type:        recType,
			Records:     records,
			TTL:         ttl,
			Description: description,
		}

		rs, err := recordsets.Create(ctx, client, zoneID, createOpts).Extract()
		if err != nil {
			return shared.ToolError("failed to create recordset: %v", err), nil
		}

		out, err := json.MarshalIndent(rs, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

func deleteRecordsetHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.DNSClient()
		if err != nil {
			return shared.ToolError("failed to get DNS client: %v", err), nil
		}

		zoneID := shared.StringParam(request, "zone_id")
		if zoneID == "" {
			return shared.ToolError("zone_id is required"), nil
		}
		if errResult := shared.ValidateUUID(zoneID, "zone_id"); errResult != nil {
			return errResult, nil
		}

		rrsetID := shared.StringParam(request, "recordset_id")
		if rrsetID == "" {
			return shared.ToolError("recordset_id is required"), nil
		}
		if errResult := shared.ValidateUUID(rrsetID, "recordset_id"); errResult != nil {
			return errResult, nil
		}

		// Fetch recordset for preview
		rs, err := recordsets.Get(ctx, client, zoneID, rrsetID).Extract()
		if err != nil {
			return shared.ToolError("failed to get recordset %s: %v", rrsetID, err), nil
		}

		preview := fmt.Sprintf("Will DELETE recordset '%s' (%s: [%s]) from zone %s",
			rs.Name, rs.Type, strings.Join(rs.Records, ", "), zoneID)
		if result := shared.RequireConfirmation(request, preview); result != nil {
			return result, nil
		}

		err = recordsets.Delete(ctx, client, zoneID, rrsetID).ExtractErr()
		if err != nil {
			return shared.ToolError("failed to delete recordset %s: %v", rrsetID, err), nil
		}

		return shared.ToolResult(fmt.Sprintf("Successfully deleted recordset %s from zone %s", rrsetID, zoneID)), nil
	}
}
