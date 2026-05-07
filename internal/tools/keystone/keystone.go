// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

// Package keystone provides MCP tools for OpenStack Identity (Keystone) operations.
package keystone

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gophercloud/gophercloud/v2/openstack/identity/v3/applicationcredentials"
	"github.com/gophercloud/gophercloud/v2/openstack/identity/v3/domains"
	"github.com/gophercloud/gophercloud/v2/openstack/identity/v3/projects"
	"github.com/gophercloud/gophercloud/v2/openstack/identity/v3/roles"
	"github.com/gophercloud/gophercloud/v2/openstack/identity/v3/tokens"
	"github.com/gophercloud/gophercloud/v2/openstack/identity/v3/users"
	"github.com/gophercloud/gophercloud/v2/pagination"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/notque/openstack-mcp-server/internal/auth"
	"github.com/notque/openstack-mcp-server/internal/tools/shared"
)

// Register adds all Keystone tools to the MCP server.
// When readOnly is true, mutating tools (create/delete credentials) are not registered.
func Register(s *mcpserver.MCPServer, provider *auth.Provider, readOnly bool) {
	s.AddTool(listProjectsTool, listProjectsHandler(provider))
	s.AddTool(tokenInfoTool, tokenInfoHandler(provider))
	s.AddTool(listAppCredentialsTool, listAppCredentialsHandler(provider))
	s.AddTool(listDomainsTool, listDomainsHandler(provider))
	s.AddTool(listUsersTool, listUsersHandler(provider))
	s.AddTool(listRolesTool, listRolesHandler(provider))
	if !readOnly {
		s.AddTool(createAppCredentialTool, createAppCredentialHandler(provider))
		s.AddTool(deleteAppCredentialTool, deleteAppCredentialHandler(provider))
	}
}

// --- Tool Definitions ---

var listProjectsTool = mcp.NewTool("keystone_list_projects",
	mcp.WithDescription("List projects (tenants) accessible to the current user. Returns project ID, name, domain, and enabled status."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithString("domain_id", mcp.Description("Filter by domain ID")),
	mcp.WithString("name", mcp.Description("Filter by project name")),
)

var tokenInfoTool = mcp.NewTool("keystone_token_info",
	mcp.WithDescription("Get information about the current authentication context: user, project, domain, roles, and service catalog. Note: the actual token value is never exposed."),
	mcp.WithReadOnlyHintAnnotation(true),
)

var createAppCredentialTool = mcp.NewTool("keystone_create_application_credential",
	mcp.WithDescription("Create an application credential for the current user. Application credentials allow authentication without exposing your main password — ideal for MCP server configuration. IMPORTANT: The secret is only shown once at creation time. Save it immediately. Best practice: call keystone_list_application_credentials first to check for existing credentials before creating a new one."),
	mcp.WithDestructiveHintAnnotation(true),
	mcp.WithString("name", mcp.Required(), mcp.Description("Name for the application credential (must be unique per user)")),
	mcp.WithString("description", mcp.Description("Description of the credential's purpose (e.g., 'MCP server access for project X')")),
	mcp.WithString("expires_at", mcp.Description("Expiration time in RFC3339 format (e.g., '2025-12-31T23:59:59Z'). If omitted, the credential does not expire.")),
	mcp.WithString("roles", mcp.Description("Comma-separated list of role names to assign (subset of current roles). If omitted, all current roles are inherited.")),
	mcp.WithBoolean("confirmed", mcp.Description("Set to true to execute. Without this, returns a preview of the action.")),
)

var listAppCredentialsTool = mcp.NewTool("keystone_list_application_credentials",
	mcp.WithDescription("List application credentials for the current user. Shows ID, name, description, roles, and expiration. Secrets are never shown (only available at creation time)."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithString("name", mcp.Description("Filter by application credential name")),
)

var deleteAppCredentialTool = mcp.NewTool("keystone_delete_application_credential",
	mcp.WithDescription("Delete an application credential by ID. This immediately revokes the credential — any services using it will lose access."),
	mcp.WithDestructiveHintAnnotation(true),
	mcp.WithString("id", mcp.Required(), mcp.Description("The UUID of the application credential to delete")),
	mcp.WithBoolean("confirmed", mcp.Description("Set to true to execute. Without this, returns a preview of the action.")),
)

var listDomainsTool = mcp.NewTool("keystone_list_domains",
	mcp.WithDescription("List identity domains. Returns domain ID, name, description, and enabled status."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithString("name", mcp.Description("Filter by domain name")),
)

var listUsersTool = mcp.NewTool("keystone_list_users",
	mcp.WithDescription("List users in the identity service. Returns user ID, name, domain_id, enabled, and description."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithString("domain_id", mcp.Description("Filter by domain UUID")),
	mcp.WithString("name", mcp.Description("Filter by username")),
)

var listRolesTool = mcp.NewTool("keystone_list_roles",
	mcp.WithDescription("List roles in the identity service. Returns role ID, name, domain_id, and description."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithString("domain_id", mcp.Description("Filter by domain UUID")),
	mcp.WithString("name", mcp.Description("Filter by role name")),
)

// --- Handlers ---

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

		out, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
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

		// SECURITY: Only expose metadata about the auth context, NEVER the token ID.
		// The token value stays in server memory and is never sent to the LLM.
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
		if tokenRoles, err := result.ExtractRoles(); err == nil {
			info["roles"] = tokenRoles
		}
		if catalog, err := result.ExtractServiceCatalog(); err == nil {
			info["service_catalog"] = catalog
		}

		out, err := json.MarshalIndent(info, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

func createAppCredentialHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.IdentityClient()
		if err != nil {
			return shared.ToolError("failed to get identity client: %v", err), nil
		}

		userID := provider.UserID()
		if userID == "" {
			return shared.ToolError("unable to determine user ID from authentication context"), nil
		}

		name := shared.StringParam(request, "name")
		if name == "" {
			return shared.ToolError("name is required"), nil
		}

		preview := fmt.Sprintf("Will CREATE application credential '%s' for the current user", name)
		if result := shared.RequireConfirmation(request, preview); result != nil {
			return result, nil
		}

		createOpts := applicationcredentials.CreateOpts{
			Name:        name,
			Description: shared.StringParam(request, "description"),
		}

		// Parse optional expiration
		if expiresStr := shared.StringParam(request, "expires_at"); expiresStr != "" {
			t, err := time.Parse(time.RFC3339, expiresStr)
			if err != nil {
				return shared.ToolError("invalid expires_at format (use RFC3339, e.g., 2025-12-31T23:59:59Z): %v", err), nil
			}
			createOpts.ExpiresAt = &t
		}

		// Parse optional roles (comma-separated names)
		if rolesStr := shared.StringParam(request, "roles"); rolesStr != "" {
			for roleName := range strings.SplitSeq(rolesStr, ",") {
				roleName = strings.TrimSpace(roleName)
				if roleName != "" {
					createOpts.Roles = append(createOpts.Roles, applicationcredentials.Role{Name: roleName})
				}
			}
		}

		appCred, err := applicationcredentials.Create(ctx, client, userID, createOpts).Extract()
		if err != nil {
			return shared.ToolError("failed to create application credential: %v", err), nil
		}

		// Build response with all relevant fields including the secret.
		// SECURITY NOTE: We use ToolResultRaw here because the secret is the PURPOSE
		// of this tool — it is only visible at creation time and the user must save it.
		result := map[string]any{
			"id":          appCred.ID,
			"name":        appCred.Name,
			"description": appCred.Description,
			"secret":      appCred.Secret,
			"project_id":  appCred.ProjectID,
			"expires_at":  appCred.ExpiresAt,
			"roles":       appCred.Roles,
		}

		out, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}

		// Include setup instructions for the user
		instructions := fmt.Sprintf(`%s

--- SAVE THIS SECRET NOW ---
The secret above is only shown once. To use this application credential with the MCP server,
configure your Claude Code settings with:

  "OS_APPLICATION_CREDENTIAL_ID": "%s",
  "OS_APPLICATION_CREDENTIAL_SECRET": "<the secret above>"

Or for secure storage, save the secret to your system keychain:
  security add-generic-password -a "%s" -s "openstack-appcred" -w "<the secret above>"

Then use:
  "OS_APPLICATION_CREDENTIAL_ID": "%s",
  "OS_APPCRED_SECRET_CMD": "security find-generic-password -a %s -s openstack-appcred -w"
`, string(out), appCred.ID, appCred.Name, appCred.ID, appCred.Name)

		return shared.ToolResultRaw(instructions), nil
	}
}

func listAppCredentialsHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.IdentityClient()
		if err != nil {
			return shared.ToolError("failed to get identity client: %v", err), nil
		}

		userID := provider.UserID()
		if userID == "" {
			return shared.ToolError("unable to determine user ID from authentication context"), nil
		}

		listOpts := applicationcredentials.ListOpts{
			Name: shared.StringParam(request, "name"),
		}

		var result []map[string]any
		err = applicationcredentials.List(client, userID, listOpts).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
			creds, err := applicationcredentials.ExtractApplicationCredentials(page)
			if err != nil {
				return false, err
			}
			for _, c := range creds {
				result = append(result, map[string]any{
					"id":          c.ID,
					"name":        c.Name,
					"description": c.Description,
					"project_id":  c.ProjectID,
					"expires_at":  c.ExpiresAt,
					"roles":       c.Roles,
				})
			}
			return true, nil
		})
		if err != nil {
			return shared.ToolError("failed to list application credentials: %v", err), nil
		}

		if result == nil {
			result = []map[string]any{}
		}

		out, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

func deleteAppCredentialHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.IdentityClient()
		if err != nil {
			return shared.ToolError("failed to get identity client: %v", err), nil
		}

		userID := provider.UserID()
		if userID == "" {
			return shared.ToolError("unable to determine user ID from authentication context"), nil
		}

		id := shared.StringParam(request, "id")
		if id == "" {
			return shared.ToolError("id is required"), nil
		}
		if errResult := shared.ValidateUUID(id, "id"); errResult != nil {
			return errResult, nil
		}

		preview := fmt.Sprintf("Will DELETE application credential %s — any services using it will immediately lose access", id)
		if result := shared.RequireConfirmation(request, preview); result != nil {
			return result, nil
		}

		err = applicationcredentials.Delete(ctx, client, userID, id).ExtractErr()
		if err != nil {
			return shared.ToolError("failed to delete application credential %s: %v", id, err), nil
		}

		return shared.ToolResult(fmt.Sprintf("Successfully deleted application credential %s. Any services using this credential will immediately lose access.", id)), nil
	}
}

func listDomainsHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.IdentityClient()
		if err != nil {
			return shared.ToolError("failed to get identity client: %v", err), nil
		}

		opts := domains.ListOpts{
			Name: shared.StringParam(request, "name"),
		}

		var result []map[string]any
		err = domains.List(client, opts).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
			domainList, err := domains.ExtractDomains(page)
			if err != nil {
				return false, err
			}
			for _, d := range domainList {
				result = append(result, map[string]any{
					"id":          d.ID,
					"name":        d.Name,
					"description": d.Description,
					"enabled":     d.Enabled,
				})
			}
			return true, nil
		})
		if err != nil {
			return shared.ToolError("failed to list domains: %v", err), nil
		}

		out, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

func listUsersHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.IdentityClient()
		if err != nil {
			return shared.ToolError("failed to get identity client: %v", err), nil
		}

		opts := users.ListOpts{
			Name: shared.StringParam(request, "name"),
		}
		if v := shared.StringParam(request, "domain_id"); v != "" {
			if errResult := shared.ValidateUUID(v, "domain_id"); errResult != nil {
				return errResult, nil
			}
			opts.DomainID = v
		}

		var result []map[string]any
		err = users.List(client, opts).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
			userList, err := users.ExtractUsers(page)
			if err != nil {
				return false, err
			}
			for _, u := range userList {
				// SECURITY: Only expose safe fields. Password and Options are intentionally omitted.
				result = append(result, map[string]any{
					"id":          u.ID,
					"name":        u.Name,
					"domain_id":   u.DomainID,
					"enabled":     u.Enabled,
					"description": u.Description,
				})
			}
			return true, nil
		})
		if err != nil {
			return shared.ToolError("failed to list users: %v", err), nil
		}

		out, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

func listRolesHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.IdentityClient()
		if err != nil {
			return shared.ToolError("failed to get identity client: %v", err), nil
		}

		opts := roles.ListOpts{
			Name: shared.StringParam(request, "name"),
		}
		if v := shared.StringParam(request, "domain_id"); v != "" {
			if errResult := shared.ValidateUUID(v, "domain_id"); errResult != nil {
				return errResult, nil
			}
			opts.DomainID = v
		}

		var result []map[string]any
		err = roles.List(client, opts).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
			roleList, err := roles.ExtractRoles(page)
			if err != nil {
				return false, err
			}
			for _, r := range roleList {
				result = append(result, map[string]any{
					"id":          r.ID,
					"name":        r.Name,
					"domain_id":   r.DomainID,
					"description": r.Description,
				})
			}
			return true, nil
		})
		if err != nil {
			return shared.ToolError("failed to list roles: %v", err), nil
		}

		out, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}
