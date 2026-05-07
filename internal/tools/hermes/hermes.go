// Package hermes provides MCP tools for SAP CC Hermes (Audit) service.
// Hermes provides centralized audit event access in CADF format.
package hermes

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/notque/openstack-mcp-server/internal/auth"
	"github.com/notque/openstack-mcp-server/internal/tools/shared"
)

// Register adds all Hermes tools to the MCP server.
func Register(s *mcpserver.MCPServer, provider *auth.Provider) {
	s.AddTool(listEventsTool, listEventsHandler(provider))
	s.AddTool(getEventTool, getEventHandler(provider))
	s.AddTool(listAttributesTool, listAttributesHandler(provider))
}

var listEventsTool = mcp.NewTool("hermes_list_events",
	mcp.WithDescription("List audit events from the Hermes audit trail. Events are in CADF format covering all OpenStack and SAP CC service actions."),
	mcp.WithString("target_type", mcp.Description("Filter by target resource type (e.g., 'compute/server', 'network/port', 'identity/project')")),
	mcp.WithString("target_id", mcp.Description("Filter by target resource UUID")),
	mcp.WithString("initiator_name", mcp.Description("Filter by who performed the action (username)")),
	mcp.WithString("action", mcp.Description("Filter by action (e.g., 'create', 'update', 'delete')")),
	mcp.WithString("outcome", mcp.Description("Filter by outcome (e.g., 'success', 'failure')")),
	mcp.WithString("time_gte", mcp.Description("Filter events after this time (RFC3339 format, e.g. '2024-01-01T00:00:00Z')")),
	mcp.WithString("time_lte", mcp.Description("Filter events before this time (RFC3339 format)")),
	mcp.WithNumber("limit", mcp.Description("Maximum events to return (default: 50)")),
	mcp.WithString("sort", mcp.Description("Sort field and direction (e.g., 'time:desc')")),
)

var getEventTool = mcp.NewTool("hermes_get_event",
	mcp.WithDescription("Get a specific audit event by its ID. Returns full CADF event details."),
	mcp.WithString("event_id", mcp.Required(), mcp.Description("The UUID of the audit event")),
)

var listAttributesTool = mcp.NewTool("hermes_list_attributes",
	mcp.WithDescription("List available attribute values for filtering audit events (e.g., all known target_types, actions, or observers)."),
	mcp.WithString("attribute", mcp.Required(), mcp.Description("Attribute to list values for: 'target_type', 'action', 'outcome', 'observer_type', 'initiator_type'")),
)

func listEventsHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.HermesClient()
		if err != nil {
			return shared.ToolError("failed to get hermes client: %v", err), nil
		}

		query := make(map[string]string)
		for _, key := range []string{"target_type", "target_id", "initiator_name", "action", "outcome", "sort"} {
			if v := shared.StringParam(request, key); v != "" {
				query[key] = v
			}
		}
		if v := shared.StringParam(request, "time_gte"); v != "" {
			query["time"] = fmt.Sprintf("gte:%s", v)
			if lte := shared.StringParam(request, "time_lte"); lte != "" {
				query["time"] += fmt.Sprintf(",lte:%s", lte)
			}
		}

		limit := int(shared.NumberParam(request, "limit"))
		if limit <= 0 {
			limit = 50
		}
		query["limit"] = fmt.Sprintf("%d", limit)

		url := client.ResourceBase + "v1/events"
		sep := "?"
		for k, v := range query {
			url += fmt.Sprintf("%s%s=%s", sep, k, v)
			sep = "&"
		}

		var body any
		//nolint:bodyclose
		_, err = client.Get(ctx, url, &body, &gophercloud.RequestOpts{
			OkCodes: []int{http.StatusOK},
		})
		if err != nil {
			return shared.ToolError("failed to list hermes events: %v", err), nil
		}

		out, _ := json.MarshalIndent(body, "", "  ")
		return shared.ToolResult(string(out)), nil
	}
}

func getEventHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.HermesClient()
		if err != nil {
			return shared.ToolError("failed to get hermes client: %v", err), nil
		}

		eventID := shared.StringParam(request, "event_id")
		if eventID == "" {
			return shared.ToolError("event_id is required"), nil
		}

		url := client.ResourceBase + "v1/events/" + eventID

		var body any
		//nolint:bodyclose
		_, err = client.Get(ctx, url, &body, &gophercloud.RequestOpts{
			OkCodes: []int{http.StatusOK},
		})
		if err != nil {
			return shared.ToolError("failed to get hermes event %s: %v", eventID, err), nil
		}

		out, _ := json.MarshalIndent(body, "", "  ")
		return shared.ToolResult(string(out)), nil
	}
}

func listAttributesHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.HermesClient()
		if err != nil {
			return shared.ToolError("failed to get hermes client: %v", err), nil
		}

		attr := shared.StringParam(request, "attribute")
		if attr == "" {
			return shared.ToolError("attribute is required"), nil
		}

		url := client.ResourceBase + "v1/attributes/" + attr

		var body any
		//nolint:bodyclose
		_, err = client.Get(ctx, url, &body, &gophercloud.RequestOpts{
			OkCodes: []int{http.StatusOK},
		})
		if err != nil {
			return shared.ToolError("failed to list hermes attributes for %s: %v", attr, err), nil
		}

		out, _ := json.MarshalIndent(body, "", "  ")
		return shared.ToolResult(string(out)), nil
	}
}
