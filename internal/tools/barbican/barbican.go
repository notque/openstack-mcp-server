// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

// Package barbican provides MCP tools for OpenStack Key Manager (Barbican) operations.
package barbican

import (
	"context"
	"encoding/json"

	"github.com/gophercloud/gophercloud/v2/openstack/keymanager/v1/containers"
	"github.com/gophercloud/gophercloud/v2/openstack/keymanager/v1/orders"
	"github.com/gophercloud/gophercloud/v2/openstack/keymanager/v1/secrets"
	"github.com/gophercloud/gophercloud/v2/pagination"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/notque/openstack-mcp-server/internal/auth"
	"github.com/notque/openstack-mcp-server/internal/tools/shared"
)

// Register adds all Barbican tools to the MCP server.
// The admin parameter is accepted for interface consistency but currently unused.
func Register(s *mcpserver.MCPServer, provider *auth.Provider, _ bool) {
	s.AddTool(listSecretsTool, listSecretsHandler(provider))
	s.AddTool(getSecretTool, getSecretHandler(provider))
	s.AddTool(listContainersTool, listContainersHandler(provider))
	s.AddTool(getContainerTool, getContainerHandler(provider))
	s.AddTool(listOrdersTool, listOrdersHandler(provider))
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
		if errResult := shared.ValidateUUID(secretID, "secret_id"); errResult != nil {
			return errResult, nil
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

// --- Containers ---

var listContainersTool = mcp.NewTool("barbican_list_containers",
	mcp.WithDescription("List secret containers (certificate bundles, RSA key pairs, etc.). Returns container_ref, name, type, status, secret_refs, created, and updated."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithString("name", mcp.Description("Filter by container name")),
	mcp.WithString("type", mcp.Description("Filter by container type (generic, rsa, certificate)")),
)

func listContainersHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.KeyManagerClient()
		if err != nil {
			return shared.ToolError("failed to get key manager client: %v", err), nil
		}

		opts := containers.ListOpts{
			Name: shared.StringParam(request, "name"),
		}
		// NOTE: Type filtering is client-side; gophercloud containers.ListOpts does not support it.
		typeFilter := shared.StringParam(request, "type")

		result := make([]map[string]any, 0)
		err = containers.List(client, opts).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
			containerList, err := containers.ExtractContainers(page)
			if err != nil {
				return false, err
			}
			for _, c := range containerList {
				if typeFilter != "" && c.Type != typeFilter {
					continue
				}
				result = append(result, map[string]any{
					"container_ref": c.ContainerRef,
					"name":          c.Name,
					"type":          c.Type,
					"status":        c.Status,
					"secret_refs":   c.SecretRefs,
					"created":       c.Created,
					"updated":       c.Updated,
				})
			}
			return true, nil
		})
		if err != nil {
			return shared.ToolError("failed to list containers: %v", err), nil
		}

		out, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

var getContainerTool = mcp.NewTool("barbican_get_container",
	mcp.WithDescription("Get details of a specific secret container. Returns metadata only (container_ref, name, type, status, secret_refs, consumers, created, updated). Secret payloads are never exposed."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithString("container_id", mcp.Required(), mcp.Description("The UUID of the container to retrieve")),
)

func getContainerHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.KeyManagerClient()
		if err != nil {
			return shared.ToolError("failed to get key manager client: %v", err), nil
		}

		containerID := shared.StringParam(request, "container_id")
		if containerID == "" {
			return shared.ToolError("container_id is required"), nil
		}
		if errResult := shared.ValidateUUID(containerID, "container_id"); errResult != nil {
			return errResult, nil
		}

		container, err := containers.Get(ctx, client, containerID).Extract()
		if err != nil {
			return shared.ToolError("failed to get container %s: %v", containerID, err), nil
		}

		// SECURITY: Only expose metadata fields — never the actual secrets.
		metadata := map[string]any{
			"container_ref": container.ContainerRef,
			"name":          container.Name,
			"type":          container.Type,
			"status":        container.Status,
			"secret_refs":   container.SecretRefs,
			"consumers":     container.Consumers,
			"created":       container.Created,
			"updated":       container.Updated,
		}

		out, err := json.MarshalIndent(metadata, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

// --- Orders ---

var listOrdersTool = mcp.NewTool("barbican_list_orders",
	mcp.WithDescription("List secret generation orders. Returns order_ref, status, type, secret_ref, created, and updated."),
	mcp.WithReadOnlyHintAnnotation(true),
)

func listOrdersHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.KeyManagerClient()
		if err != nil {
			return shared.ToolError("failed to get key manager client: %v", err), nil
		}

		result := make([]map[string]any, 0)
		err = orders.List(client, nil).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
			orderList, err := orders.ExtractOrders(page)
			if err != nil {
				return false, err
			}
			for _, o := range orderList {
				result = append(result, map[string]any{
					"order_ref":  o.OrderRef,
					"status":     o.Status,
					"type":       o.Type,
					"secret_ref": o.SecretRef,
					"created":    o.Created,
					"updated":    o.Updated,
				})
			}
			return true, nil
		})
		if err != nil {
			return shared.ToolError("failed to list orders: %v", err), nil
		}

		out, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}
