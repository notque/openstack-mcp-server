// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

// Package barbican provides MCP tools for OpenStack Key Manager (Barbican) operations.
package barbican

import (
	"context"
	"encoding/json"

	"github.com/gophercloud/gophercloud/v2/openstack/keymanager/v1/secrets"
	"github.com/gophercloud/gophercloud/v2/pagination"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/notque/openstack-mcp-server/internal/auth"
	"github.com/notque/openstack-mcp-server/internal/tools/shared"
)

// Register adds all Barbican tools to the MCP server.
func Register(s *mcpserver.MCPServer, provider *auth.Provider) {
	s.AddTool(listSecretsTool, listSecretsHandler(provider))
	s.AddTool(getSecretTool, getSecretHandler(provider))
}

var listSecretsTool = mcp.NewTool("barbican_list_secrets",
	mcp.WithDescription("List secrets stored in the key manager. Returns metadata only (secret_ref, name, status, secret_type, algorithm, bit_length, created, expiration). The secret payload is never returned for security."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithString("name", mcp.Description("Filter by secret name")),
	mcp.WithString("secret_type", mcp.Description("Filter by secret type (symmetric, public, private, passphrase, certificate, opaque)")),
)

var getSecretTool = mcp.NewTool("barbican_get_secret",
	mcp.WithDescription("Get metadata for a specific secret. Returns name, status, secret_type, algorithm, bit_length, mode, created, updated, expiration, and content_types. The secret payload is never returned for security."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithString("secret_id", mcp.Required(), mcp.Description("The UUID of the secret to retrieve")),
)

func listSecretsHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.KeyManagerClient()
		if err != nil {
			return shared.ToolError("failed to get key manager client: %v", err), nil
		}

		opts := secrets.ListOpts{
			Name:       shared.StringParam(request, "name"),
			SecretType: secrets.SecretType(shared.StringParam(request, "secret_type")),
		}

		var result []map[string]any
		err = secrets.List(client, opts).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
			secretList, err := secrets.ExtractSecrets(page)
			if err != nil {
				return false, err
			}
			for _, s := range secretList {
				result = append(result, map[string]any{
					"secret_ref":  s.SecretRef,
					"name":        s.Name,
					"status":      s.Status,
					"secret_type": s.SecretType,
					"algorithm":   s.Algorithm,
					"bit_length":  s.BitLength,
					"created":     s.Created,
					"expiration":  s.Expiration,
				})
			}
			return true, nil
		})
		if err != nil {
			return shared.ToolError("failed to list secrets: %v", err), nil
		}

		out, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

func getSecretHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.KeyManagerClient()
		if err != nil {
			return shared.ToolError("failed to get key manager client: %v", err), nil
		}

		secretID := shared.StringParam(request, "secret_id")
		if secretID == "" {
			return shared.ToolError("secret_id is required"), nil
		}

		secret, err := secrets.Get(ctx, client, secretID).Extract()
		if err != nil {
			return shared.ToolError("failed to get secret %s: %v", secretID, err), nil
		}

		// Return metadata only — never expose the secret payload.
		metadata := map[string]any{
			"name":          secret.Name,
			"status":        secret.Status,
			"secret_type":   secret.SecretType,
			"algorithm":     secret.Algorithm,
			"bit_length":    secret.BitLength,
			"mode":          secret.Mode,
			"created":       secret.Created,
			"updated":       secret.Updated,
			"expiration":    secret.Expiration,
			"content_types": secret.ContentTypes,
		}

		out, err := json.MarshalIndent(metadata, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}
