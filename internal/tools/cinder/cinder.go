// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

// Package cinder provides MCP tools for OpenStack Block Storage (Cinder) operations.
package cinder

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gophercloud/gophercloud/v2/openstack/blockstorage/v3/backups"
	"github.com/gophercloud/gophercloud/v2/openstack/blockstorage/v3/quotasets"
	"github.com/gophercloud/gophercloud/v2/openstack/blockstorage/v3/services"
	"github.com/gophercloud/gophercloud/v2/openstack/blockstorage/v3/snapshots"
	"github.com/gophercloud/gophercloud/v2/openstack/blockstorage/v3/transfers"
	"github.com/gophercloud/gophercloud/v2/openstack/blockstorage/v3/volumes"
	"github.com/gophercloud/gophercloud/v2/openstack/blockstorage/v3/volumetypes"
	"github.com/gophercloud/gophercloud/v2/pagination"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/notque/openstack-mcp-server/internal/auth"
	"github.com/notque/openstack-mcp-server/internal/tools/shared"
)

// Register adds all Cinder tools to the MCP server.
// When readOnly is true, mutating tools are not registered.
// When admin is true, admin-only tools (services) are registered.
func Register(s *mcpserver.MCPServer, provider *auth.Provider, readOnly bool, admin bool) {
	s.AddTool(listVolumesTool, listVolumesHandler(provider))
	s.AddTool(getVolumeTool, getVolumeHandler(provider))
	s.AddTool(listSnapshotsTool, listSnapshotsHandler(provider))
	s.AddTool(getSnapshotTool, getSnapshotHandler(provider))
	s.AddTool(listVolumeTypesTool, listVolumeTypesHandler(provider))
	s.AddTool(getQuotasTool, getQuotasHandler(provider))
	s.AddTool(listBackupsTool, listBackupsHandler(provider))
	s.AddTool(listTransfersTool, listTransfersHandler(provider))

	if admin {
		s.AddTool(listServicesTool, listServicesHandler(provider))
	}

	if !readOnly {
		s.AddTool(createVolumeTool, createVolumeHandler(provider))
		s.AddTool(deleteVolumeTool, deleteVolumeHandler(provider))
	}
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

// --- Read tool: quotas ---

var getQuotasTool = mcp.NewTool("cinder_get_quotas",
	mcp.WithDescription("Get block storage quota usage for a project. Shows limits and current usage for volumes, snapshots, gigabytes, and backups."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithString("project_id", mcp.Required(), mcp.Description("The UUID of the project to get quotas for")),
)

func getQuotasHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.BlockStorageClient()
		if err != nil {
			return shared.ToolError("failed to get block storage client: %v", err), nil
		}

		projectID := shared.StringParam(request, "project_id")
		if projectID == "" {
			return shared.ToolError("project_id is required"), nil
		}
		if errResult := shared.ValidateUUID(projectID, "project_id"); errResult != nil {
			return errResult, nil
		}

		usage, err := quotasets.GetUsage(ctx, client, projectID).Extract()
		if err != nil {
			return shared.ToolError("failed to get quotas for project %s: %v", projectID, err), nil
		}

		out, err := json.MarshalIndent(usage, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

// --- Backups ---

var listBackupsTool = mcp.NewTool("cinder_list_backups",
	mcp.WithDescription("List volume backups in the current project. Returns backup ID, name, status, volume ID, size, availability zone, and created_at."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithString("volume_id", mcp.Description("Filter by volume UUID")),
	mcp.WithString("status", mcp.Description("Filter by backup status")),
	mcp.WithString("name", mcp.Description("Filter by backup name")),
)

func listBackupsHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.BlockStorageClient()
		if err != nil {
			return shared.ToolError("failed to get block storage client: %v", err), nil
		}

		opts := backups.ListOpts{
			Name:   shared.StringParam(request, "name"),
			Status: shared.StringParam(request, "status"),
		}
		if v := shared.StringParam(request, "volume_id"); v != "" {
			if errResult := shared.ValidateUUID(v, "volume_id"); errResult != nil {
				return errResult, nil
			}
			opts.VolumeID = v
		}

		result := make([]map[string]any, 0)
		err = backups.List(client, opts).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
			allBackups, err := backups.ExtractBackups(page)
			if err != nil {
				return false, err
			}
			for _, b := range allBackups {
				result = append(result, map[string]any{
					"id":                b.ID,
					"name":              b.Name,
					"status":            b.Status,
					"volume_id":         b.VolumeID,
					"size":              b.Size,
					"availability_zone": b.AvailabilityZone,
					"created_at":        b.CreatedAt,
				})
			}
			return true, nil
		})
		if err != nil {
			return shared.ToolError("failed to list backups: %v", err), nil
		}

		out, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

// --- Transfers ---

var listTransfersTool = mcp.NewTool("cinder_list_transfers",
	mcp.WithDescription("List volume transfer requests in the current project. Returns transfer ID, name, volume ID, and created_at."),
	mcp.WithReadOnlyHintAnnotation(true),
)

func listTransfersHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.BlockStorageClient()
		if err != nil {
			return shared.ToolError("failed to get block storage client: %v", err), nil
		}

		result := make([]map[string]any, 0)
		err = transfers.List(client, nil).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
			allTransfers, err := transfers.ExtractTransfers(page)
			if err != nil {
				return false, err
			}
			for _, t := range allTransfers {
				result = append(result, map[string]any{
					"id":         t.ID,
					"name":       t.Name,
					"volume_id":  t.VolumeID,
					"created_at": t.CreatedAt,
				})
			}
			return true, nil
		})
		if err != nil {
			return shared.ToolError("failed to list transfers: %v", err), nil
		}

		out, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

// --- Admin tools ---

var listServicesTool = mcp.NewTool("cinder_list_services",
	mcp.WithDescription("[Admin] List block storage services. Requires admin role."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithString("binary", mcp.Description("Filter by service binary name (e.g., 'cinder-volume')")),
	mcp.WithString("host", mcp.Description("Filter by host name")),
)

func listServicesHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.BlockStorageClient()
		if err != nil {
			return shared.ToolError("failed to get block storage client: %v", err), nil
		}

		opts := services.ListOpts{
			Binary: shared.StringParam(request, "binary"),
			Host:   shared.StringParam(request, "host"),
		}

		result := make([]map[string]any, 0)
		err = services.List(client, opts).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
			allServices, err := services.ExtractServices(page)
			if err != nil {
				return false, err
			}
			for _, svc := range allServices {
				result = append(result, map[string]any{
					"binary":     svc.Binary,
					"host":       svc.Host,
					"zone":       svc.Zone,
					"status":     svc.Status,
					"state":      svc.State,
					"updated_at": svc.UpdatedAt,
				})
			}
			return true, nil
		})
		if err != nil {
			return shared.ToolError("failed to list services: %v", err), nil
		}

		out, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

// --- Write tools ---

var createVolumeTool = mcp.NewTool("cinder_create_volume",
	mcp.WithDescription("Create a new block storage volume."),
	mcp.WithDestructiveHintAnnotation(true),
	mcp.WithString("name", mcp.Description("Name for the new volume")),
	mcp.WithNumber("size", mcp.Required(), mcp.Description("Size of the volume in GiB (must be > 0)")),
	mcp.WithString("volume_type", mcp.Description("Volume type (e.g., 'vmware_hdd', 'vmware_ssd')")),
	mcp.WithString("availability_zone", mcp.Description("Availability zone for the volume")),
	mcp.WithString("description", mcp.Description("Description of the volume")),
	mcp.WithString("snapshot_id", mcp.Description("UUID of a snapshot to create the volume from")),
	mcp.WithString("source_volume_id", mcp.Description("UUID of an existing volume to clone")),
	mcp.WithBoolean("confirmed", mcp.Description("Set to true to execute. Without this, returns a preview of the action.")),
)

var deleteVolumeTool = mcp.NewTool("cinder_delete_volume",
	mcp.WithDescription("Delete a block storage volume. Volume must not be attached to any server."),
	mcp.WithDestructiveHintAnnotation(true),
	mcp.WithString("volume_id", mcp.Required(), mcp.Description("The UUID of the volume to delete")),
	mcp.WithBoolean("confirmed", mcp.Description("Set to true to execute. Without this, returns a preview of the action.")),
)

func createVolumeHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.BlockStorageClient()
		if err != nil {
			return shared.ToolError("failed to get block storage client: %v", err), nil
		}

		size := int(shared.NumberParam(request, "size"))
		if size <= 0 {
			return shared.ToolError("size must be greater than 0"), nil
		}

		name := shared.StringParam(request, "name")
		volumeType := shared.StringParam(request, "volume_type")
		az := shared.StringParam(request, "availability_zone")
		description := shared.StringParam(request, "description")
		snapshotID := shared.StringParam(request, "snapshot_id")
		sourceVolID := shared.StringParam(request, "source_volume_id")

		if snapshotID != "" {
			if errResult := shared.ValidateUUID(snapshotID, "snapshot_id"); errResult != nil {
				return errResult, nil
			}
		}
		if sourceVolID != "" {
			if errResult := shared.ValidateUUID(sourceVolID, "source_volume_id"); errResult != nil {
				return errResult, nil
			}
		}

		nameDisplay := name
		if nameDisplay == "" {
			nameDisplay = "(unnamed)"
		}
		typeDisplay := volumeType
		if typeDisplay == "" {
			typeDisplay = "default"
		}
		azDisplay := az
		if azDisplay == "" {
			azDisplay = "default"
		}
		preview := fmt.Sprintf("Will CREATE volume '%s', %dGiB, type: %s, AZ: %s",
			nameDisplay, size, typeDisplay, azDisplay)
		if result := shared.RequireConfirmation(request, preview); result != nil {
			return result, nil
		}

		createOpts := volumes.CreateOpts{
			Name:             name,
			Size:             size,
			VolumeType:       volumeType,
			AvailabilityZone: az,
			Description:      description,
			SnapshotID:       snapshotID,
			SourceVolID:      sourceVolID,
		}

		vol, err := volumes.Create(ctx, client, createOpts, nil).Extract()
		if err != nil {
			return shared.ToolError("failed to create volume: %v", err), nil
		}

		out, err := json.MarshalIndent(vol, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

func deleteVolumeHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
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

		// Fetch volume to check status and build preview
		vol, err := volumes.Get(ctx, client, volumeID).Extract()
		if err != nil {
			return shared.ToolError("failed to get volume %s: %v", volumeID, err), nil
		}

		if vol.Status == "in-use" {
			return shared.ToolError("cannot delete volume %s: currently attached to server(s) (status: in-use). Detach first.", volumeID), nil
		}

		preview := fmt.Sprintf("Will DELETE volume '%s' (%s), %dGiB, status: %s",
			vol.Name, vol.ID, vol.Size, vol.Status)
		if result := shared.RequireConfirmation(request, preview); result != nil {
			return result, nil
		}

		err = volumes.Delete(ctx, client, volumeID, nil).ExtractErr()
		if err != nil {
			return shared.ToolError("failed to delete volume %s: %v", volumeID, err), nil
		}

		return shared.ToolResult("Successfully deleted volume " + volumeID), nil
	}
}
