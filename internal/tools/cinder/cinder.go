// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

// Package cinder provides MCP tools for OpenStack Block Storage (Cinder) operations.
package cinder

import (
	"context"
	"encoding/json"

	"github.com/gophercloud/gophercloud/v2/openstack/blockstorage/v3/volumes"
	"github.com/gophercloud/gophercloud/v2/pagination"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/notque/openstack-mcp-server/internal/auth"
	"github.com/notque/openstack-mcp-server/internal/tools/shared"
)

// Register adds all Cinder tools to the MCP server.
func Register(s *mcpserver.MCPServer, provider *auth.Provider) {
	s.AddTool(listVolumesTool, listVolumesHandler(provider))
	s.AddTool(getVolumeTool, getVolumeHandler(provider))
}

var listVolumesTool = mcp.NewTool("cinder_list_volumes",
	mcp.WithDescription("List block storage volumes in the current project. Returns volume ID, name, status, size, type, and attachments."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithString("status", mcp.Description("Filter by volume status (available, in-use, error, creating, deleting)")),
	mcp.WithString("name", mcp.Description("Filter by volume name")),
)

var getVolumeTool = mcp.NewTool("cinder_get_volume",
	mcp.WithDescription("Get detailed information about a specific block storage volume."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithString("volume_id", mcp.Required(), mcp.Description("The UUID of the volume to retrieve")),
)

func listVolumesHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.BlockStorageClient()
		if err != nil {
			return shared.ToolError("failed to get block storage client: %v", err), nil
		}

		opts := volumes.ListOpts{
			Status: shared.StringParam(request, "status"),
			Name:   shared.StringParam(request, "name"),
		}

		var result []map[string]any
		err = volumes.List(client, opts).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
			vols, err := volumes.ExtractVolumes(page)
			if err != nil {
				return false, err
			}
			for _, v := range vols {
				result = append(result, map[string]any{
					"id":          v.ID,
					"name":        v.Name,
					"status":      v.Status,
					"size":        v.Size,
					"volume_type": v.VolumeType,
					"attachments": v.Attachments,
					"created_at":  v.CreatedAt,
				})
			}
			return true, nil
		})
		if err != nil {
			return shared.ToolError("failed to list volumes: %v", err), nil
		}

		out, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

func getVolumeHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.BlockStorageClient()
		if err != nil {
			return shared.ToolError("failed to get block storage client: %v", err), nil
		}

		volumeID := shared.StringParam(request, "volume_id")
		if volumeID == "" {
			return shared.ToolError("volume_id is required"), nil
		}

		vol, err := volumes.Get(ctx, client, volumeID).Extract()
		if err != nil {
			return shared.ToolError("failed to get volume %s: %v", volumeID, err), nil
		}

		out, err := json.MarshalIndent(vol, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}
