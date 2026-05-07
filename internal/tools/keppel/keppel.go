// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

// Package keppel provides MCP tools for SAP CC Keppel (Container Registry) service.
// Keppel is a multi-tenant, regionally federated container image registry.
package keppel

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

// Register adds all Keppel tools to the MCP server.
func Register(s *mcpserver.MCPServer, provider *auth.Provider) {
	s.AddTool(listAccountsTool, listAccountsHandler(provider))
	s.AddTool(listReposTool, listReposHandler(provider))
	s.AddTool(listManifestsTool, listManifestsHandler(provider))
}

var listAccountsTool = mcp.NewTool("keppel_list_accounts",
	mcp.WithDescription("List Keppel container registry accounts. Each account is a namespace with dedicated backing storage."),
)

var listReposTool = mcp.NewTool("keppel_list_repositories",
	mcp.WithDescription("List repositories within a Keppel account. Shows image repositories with manifest and tag counts."),
	mcp.WithString("account", mcp.Required(), mcp.Description("The account name to list repositories for")),
)

var listManifestsTool = mcp.NewTool("keppel_list_manifests",
	mcp.WithDescription("List manifests (image versions) in a repository. Shows tags, digest, size, and vulnerability status."),
	mcp.WithString("account", mcp.Required(), mcp.Description("The account name")),
	mcp.WithString("repository", mcp.Required(), mcp.Description("The repository name within the account")),
)

func listAccountsHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.KeppelClient()
		if err != nil {
			return shared.ToolError("failed to get keppel client: %v", err), nil
		}

		url := client.Endpoint + "keppel/v1/accounts"

		var body any
		//nolint:bodyclose
		_, err = client.Get(ctx, url, &body, &gophercloud.RequestOpts{
			OkCodes: []int{http.StatusOK},
		})
		if err != nil {
			return shared.ToolError("failed to list keppel accounts: %v", err), nil
		}

		out, err := json.MarshalIndent(body, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

func listReposHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.KeppelClient()
		if err != nil {
			return shared.ToolError("failed to get keppel client: %v", err), nil
		}

		account := shared.StringParam(request, "account")
		if account == "" {
			return shared.ToolError("account is required"), nil
		}

		url := client.Endpoint + "keppel/v1/accounts/" + account + "/repositories"

		var body any
		//nolint:bodyclose
		_, err = client.Get(ctx, url, &body, &gophercloud.RequestOpts{
			OkCodes: []int{http.StatusOK},
		})
		if err != nil {
			return shared.ToolError("failed to list repositories for account %s: %v", account, err), nil
		}

		out, err := json.MarshalIndent(body, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

func listManifestsHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.KeppelClient()
		if err != nil {
			return shared.ToolError("failed to get keppel client: %v", err), nil
		}

		account := shared.StringParam(request, "account")
		repo := shared.StringParam(request, "repository")
		if account == "" || repo == "" {
			return shared.ToolError("account and repository are required"), nil
		}

		url := client.Endpoint + "keppel/v1/accounts/" + account + "/repositories/" + repo + "/_manifests"

		var body any
		//nolint:bodyclose
		_, err = client.Get(ctx, url, &body, &gophercloud.RequestOpts{
			OkCodes: []int{http.StatusOK},
		})
		if err != nil {
			return shared.ToolError("failed to list manifests for %s/%s: %v", account, repo, err), nil
		}

		out, err := json.MarshalIndent(body, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}
