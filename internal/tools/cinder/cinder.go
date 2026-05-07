// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

// Package cinder provides MCP tools for OpenStack Block Storage (Cinder) operations.
package cinder

import (
	"context"
	"encoding/json"

	"github.com/gophercloud/gophercloud/v2/openstack/blockstorage/v3/snapshots"
	"github.com/gophercloud/gophercloud/v2/openstack/blockstorage/v3/volumes"
	"github.com/gophercloud/gophercloud/v2/openstack/blockstorage/v3/volumetypes"
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
	s.AddTool(listSnapshotsTool, listSnapshotsHandler(provider))
	s.AddTool(getSnapshotTool, getSnapshotHandler(provider))
	s.AddTool(listVolumeTypesTool, listVolumeTypesHandler(provider))
}

var listVolumesTool = mcp.NewTool("cinder_list_volumes",
	mcp.WithDescription("List block storage volumes in the current project. Returns volume ID, name, status, size, type, and attachments."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithString("status", mcp.Description("Filter by volume status (available, in-use, error, creating, deleting)")),
	mcp.WithString("name", mcp.Description("Filter by volume name")),
	mcp.WithNumber("limit", mcp.Description("Maximum number of volumes to return")),
)

var getVolumeTool = mcp.NewTool("cinder_get_volume",
	mcp.WithDescription("Get detailed information about a specific block storage volume."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithString("volume_id", mcp.Required(), mcp.Description("The UUID of the volume to retrieve")),
)

var listSnapshotsTool = mcp.NewTool("cinder_list_snapshots",
	mcp.WithDescription("List block storage snapshots in the current project. Returns snapshot ID, name, status, volume ID, size, and created_at."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithString("name", mcp.Description("Filter by snapshot name")),
	mcp.WithString("status", mcp.Description("Filter by snapshot status (available, creating, deleting, error)")),
	mcp.WithString("volume_id", mcp.Description("Filter by volume ID")),
)

var getSnapshotTool = mcp.NewTool("cinder_get_snapshot",
	mcp.WithDescription("Get detailed information about a specific block storage snapshot."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithString("snapshot_id", mcp.Required(), mcp.Description("The UUID of the snapshot to retrieve")),
)

var listVolumeTypesTool = mcp.NewTool("cinder_list_volume_types",
	mcp.WithDescription("List available block storage volume types. Returns type ID, name, description, and extra specs."),
	mcp.WithReadOnlyHintAnnotation(true),
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
		if limit := shared.NumberParam(request, "limit"); limit > 0 {
			opts.Limit = int(limit)
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
		if errResult := shared.ValidateUUID(volumeID, "volume_id"); errResult != nil {
			return errResult, nil
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

func listSnapshotsHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.BlockStorageClient()
		if err != nil {
			return shared.ToolError("failed to get block storage client: %v", err), nil
		}

		opts := snapshots.ListOpts{
			Name:   shared.StringParam(request, "name"),
			Status: shared.StringParam(request, "status"),
		}
		if v := shared.StringParam(request, "volume_id"); v != "" {
			if errResult := shared.ValidateUUID(v, "volume_id"); errResult != nil {
				return errResult, nil
			}
			opts.VolumeID = v
		}

		var result []map[string]any
		err = snapshots.List(client, opts).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
			snaps, err := snapshots.ExtractSnapshots(page)
			if err != nil {
				return false, err
			}
			for _, s := range snaps {
				result = append(result, map[string]any{
					"id":         s.ID,
					"name":       s.Name,
					"status":     s.Status,
					"volume_id":  s.VolumeID,
					"size":       s.Size,
					"created_at": s.CreatedAt,
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

func getSnapshotHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.BlockStorageClient()
		if err != nil {
			return shared.ToolError("failed to get block storage client: %v", err), nil
		}

		snapshotID := shared.StringParam(request, "snapshot_id")
		if snapshotID == "" {
			return shared.ToolError("snapshot_id is required"), nil
		}
		if errResult := shared.ValidateUUID(snapshotID, "snapshot_id"); errResult != nil {
			return errResult, nil
		}

		snap, err := snapshots.Get(ctx, client, snapshotID).Extract()
		if err != nil {
			return shared.ToolError("failed to get snapshot %s: %v", snapshotID, err), nil
		}

		out, err := json.MarshalIndent(snap, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

func listVolumeTypesHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.BlockStorageClient()
		if err != nil {
			return shared.ToolError("failed to get block storage client: %v", err), nil
		}

		var result []map[string]any
		err = volumetypes.List(client, nil).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
			vts, err := volumetypes.ExtractVolumeTypes(page)
			if err != nil {
				return false, err
			}
			for _, vt := range vts {
				result = append(result, map[string]any{
					"id":          vt.ID,
					"name":        vt.Name,
					"description": vt.Description,
					"extra_specs": vt.ExtraSpecs,
				})
			}
			return true, nil
		})
		if err != nil {
			return shared.ToolError("failed to list volume types: %v", err), nil
		}

		out, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}
