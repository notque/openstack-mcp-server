// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

// Package manila provides MCP tools for OpenStack Shared File Systems (Manila) operations.
package manila

import (
	"context"
	"encoding/json"

	"github.com/gophercloud/gophercloud/v2/openstack/sharedfilesystems/v2/securityservices"
	"github.com/gophercloud/gophercloud/v2/openstack/sharedfilesystems/v2/shareaccessrules"
	"github.com/gophercloud/gophercloud/v2/openstack/sharedfilesystems/v2/sharenetworks"
	"github.com/gophercloud/gophercloud/v2/openstack/sharedfilesystems/v2/shares"
	"github.com/gophercloud/gophercloud/v2/openstack/sharedfilesystems/v2/sharetypes"
	"github.com/gophercloud/gophercloud/v2/openstack/sharedfilesystems/v2/snapshots"
	"github.com/gophercloud/gophercloud/v2/pagination"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/notque/openstack-mcp-server/internal/auth"
	"github.com/notque/openstack-mcp-server/internal/tools/shared"
)

// Register adds all Manila tools to the MCP server.
// The admin parameter is accepted for interface consistency but currently unused.
func Register(s *mcpserver.MCPServer, provider *auth.Provider, _ bool) {
	s.AddTool(listSharesTool, listSharesHandler(provider))
	s.AddTool(getShareTool, getShareHandler(provider))
	s.AddTool(listAccessRulesTool, listAccessRulesHandler(provider))
	s.AddTool(listShareNetworksTool, listShareNetworksHandler(provider))
	s.AddTool(listSnapshotsTool, listSnapshotsHandler(provider))
	s.AddTool(listSecurityServicesTool, listSecurityServicesHandler(provider))
	s.AddTool(listShareTypesTool, listShareTypesHandler(provider))
}

var listSharesTool = mcp.NewTool("manila_list_shares",
	mcp.WithDescription("List shared file system shares in the current project. Returns share ID, name, status, protocol, size, and availability zone."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithString("name", mcp.Description("Filter by share name")),
	mcp.WithString("status", mcp.Description("Filter by share status (available, error, creating, deleting, error_deleting)")),
	mcp.WithString("share_proto", mcp.Description("Filter by share protocol (NFS, CIFS, GlusterFS, HDFS, CephFS)")),
)

var getShareTool = mcp.NewTool("manila_get_share",
	mcp.WithDescription("Get detailed information about a specific shared file system share."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithString("share_id", mcp.Required(), mcp.Description("The UUID of the share to retrieve")),
)

func listSharesHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.SharedFileSystemClient()
		if err != nil {
			return shared.ToolError("failed to get shared file system client: %v", err), nil
		}

		opts := shares.ListOpts{}
		if v := shared.StringParam(request, "name"); v != "" {
			opts.Name = v
		}
		if v := shared.StringParam(request, "status"); v != "" {
			opts.Status = v
		}

		// Manila API does not support share_proto as a query filter;
		// apply client-side filtering after extraction.
		shareProtoFilter := shared.StringParam(request, "share_proto")

		var result []map[string]any
		err = shares.ListDetail(client, opts).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
			allShares, err := shares.ExtractShares(page)
			if err != nil {
				return false, err
			}
			for _, s := range allShares {
				if shareProtoFilter != "" && s.ShareProto != shareProtoFilter {
					continue
				}
				result = append(result, map[string]any{
					"id":                s.ID,
					"name":              s.Name,
					"status":            s.Status,
					"share_proto":       s.ShareProto,
					"size":              s.Size,
					"availability_zone": s.AvailabilityZone,
					"share_type_name":   s.ShareTypeName,
					"host":              s.Host,
					"created_at":        s.CreatedAt,
				})
			}
			return true, nil
		})
		if err != nil {
			return shared.ToolError("failed to list shares: %v", err), nil
		}

		out, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

func getShareHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.SharedFileSystemClient()
		if err != nil {
			return shared.ToolError("failed to get shared file system client: %v", err), nil
		}

		shareID := shared.StringParam(request, "share_id")
		if shareID == "" {
			return shared.ToolError("share_id is required"), nil
		}
		if errResult := shared.ValidateUUID(shareID, "share_id"); errResult != nil {
			return errResult, nil
		}

		share, err := shares.Get(ctx, client, shareID).Extract()
		if err != nil {
			return shared.ToolError("failed to get share %s: %v", shareID, err), nil
		}

		out, err := json.MarshalIndent(share, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

var listAccessRulesTool = mcp.NewTool("manila_list_access_rules",
	mcp.WithDescription("List access rules for a shared file system share. Returns rule ID, access type, access to, access level, and state."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithString("share_id", mcp.Required(), mcp.Description("The UUID of the share to list access rules for")),
)

func listAccessRulesHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.SharedFileSystemClient()
		if err != nil {
			return shared.ToolError("failed to get shared file system client: %v", err), nil
		}

		shareID := shared.StringParam(request, "share_id")
		if shareID == "" {
			return shared.ToolError("share_id is required"), nil
		}
		if errResult := shared.ValidateUUID(shareID, "share_id"); errResult != nil {
			return errResult, nil
		}

		accessList, err := shareaccessrules.List(ctx, client, shareID).Extract()
		if err != nil {
			return shared.ToolError("failed to list access rules for share %s: %v", shareID, err), nil
		}

		var result []map[string]any
		for _, rule := range accessList {
			result = append(result, map[string]any{
				"id":           rule.ID,
				"access_type":  rule.AccessType,
				"access_to":    rule.AccessTo,
				"access_level": rule.AccessLevel,
				"state":        rule.State,
			})
		}

		out, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

// --- Share Networks ---

var listShareNetworksTool = mcp.NewTool("manila_list_share_networks",
	mcp.WithDescription("List share networks in the current project. Returns share network ID, name, neutron network/subnet IDs, and project ID."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithString("name", mcp.Description("Filter by share network name")),
)

func listShareNetworksHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.SharedFileSystemClient()
		if err != nil {
			return shared.ToolError("failed to get shared file system client: %v", err), nil
		}

		opts := sharenetworks.ListOpts{
			Name: shared.StringParam(request, "name"),
		}

		result := make([]map[string]any, 0)
		err = sharenetworks.ListDetail(client, opts).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
			networks, err := sharenetworks.ExtractShareNetworks(page)
			if err != nil {
				return false, err
			}
			for _, n := range networks {
				result = append(result, map[string]any{
					"id":                n.ID,
					"name":              n.Name,
					"neutron_net_id":    n.NeutronNetID,
					"neutron_subnet_id": n.NeutronSubnetID,
					"created_at":        n.CreatedAt,
					"project_id":        n.ProjectID,
				})
			}
			return true, nil
		})
		if err != nil {
			return shared.ToolError("failed to list share networks: %v", err), nil
		}

		out, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

// --- Snapshots ---

var listSnapshotsTool = mcp.NewTool("manila_list_snapshots",
	mcp.WithDescription("List share snapshots. Returns snapshot ID, name, status, share ID, share size, share protocol, and created_at."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithString("share_id", mcp.Description("Filter by share UUID")),
	mcp.WithString("status", mcp.Description("Filter by snapshot status (available, error, creating, deleting)")),
)

func listSnapshotsHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.SharedFileSystemClient()
		if err != nil {
			return shared.ToolError("failed to get shared file system client: %v", err), nil
		}

		opts := snapshots.ListOpts{}
		if v := shared.StringParam(request, "share_id"); v != "" {
			if errResult := shared.ValidateUUID(v, "share_id"); errResult != nil {
				return errResult, nil
			}
			opts.ShareID = v
		}
		if v := shared.StringParam(request, "status"); v != "" {
			opts.Status = v
		}

		result := make([]map[string]any, 0)
		err = snapshots.ListDetail(client, opts).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
			snapshotList, err := snapshots.ExtractSnapshots(page)
			if err != nil {
				return false, err
			}
			for _, s := range snapshotList {
				result = append(result, map[string]any{
					"id":          s.ID,
					"name":        s.Name,
					"status":      s.Status,
					"share_id":    s.ShareID,
					"share_size":  s.ShareSize,
					"share_proto": s.ShareProto,
					"created_at":  s.CreatedAt,
				})
			}
			return true, nil
		})
		if err != nil {
			return shared.ToolError("failed to list snapshots: %v", err), nil
		}

		out, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

// --- Security Services ---

var listSecurityServicesTool = mcp.NewTool("manila_list_security_services",
	mcp.WithDescription("List security services (LDAP/Kerberos/Active Directory) configured for share networks."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithString("type", mcp.Description("Filter by security service type (ldap, kerberos, active_directory)")),
	mcp.WithString("name", mcp.Description("Filter by security service name")),
)

func listSecurityServicesHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.SharedFileSystemClient()
		if err != nil {
			return shared.ToolError("failed to get shared file system client: %v", err), nil
		}

		opts := securityservices.ListOpts{
			Name: shared.StringParam(request, "name"),
		}
		if v := shared.StringParam(request, "type"); v != "" {
			opts.Type = securityservices.SecurityServiceType(v)
		}

		result := make([]map[string]any, 0)
		err = securityservices.List(client, opts).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
			svcList, err := securityservices.ExtractSecurityServices(page)
			if err != nil {
				return false, err
			}
			for _, s := range svcList {
				// SECURITY: Do NOT include Password field.
				result = append(result, map[string]any{
					"id":     s.ID,
					"name":   s.Name,
					"type":   s.Type,
					"status": s.Status,
					"dns_ip": s.DNSIP,
					"server": s.Server,
					"domain": s.Domain,
				})
			}
			return true, nil
		})
		if err != nil {
			return shared.ToolError("failed to list security services: %v", err), nil
		}

		out, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

// --- Share Types ---

var listShareTypesTool = mcp.NewTool("manila_list_share_types",
	mcp.WithDescription("List available share types. Returns share type ID, name, public visibility, and extra specifications."),
	mcp.WithReadOnlyHintAnnotation(true),
)

func listShareTypesHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.SharedFileSystemClient()
		if err != nil {
			return shared.ToolError("failed to get shared file system client: %v", err), nil
		}

		result := make([]map[string]any, 0)
		err = sharetypes.List(client, nil).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
			typeList, err := sharetypes.ExtractShareTypes(page)
			if err != nil {
				return false, err
			}
			for _, t := range typeList {
				result = append(result, map[string]any{
					"id":          t.ID,
					"name":        t.Name,
					"is_public":   t.IsPublic,
					"extra_specs": t.ExtraSpecs,
				})
			}
			return true, nil
		})
		if err != nil {
			return shared.ToolError("failed to list share types: %v", err), nil
		}

		out, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}
