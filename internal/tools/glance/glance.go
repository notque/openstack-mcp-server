// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

// Package glance provides MCP tools for OpenStack Image Service (Glance) operations.
package glance

import (
	"context"
	"encoding/json"

	"github.com/gophercloud/gophercloud/v2/openstack/image/v2/images"
	"github.com/gophercloud/gophercloud/v2/openstack/image/v2/members"
	"github.com/gophercloud/gophercloud/v2/openstack/image/v2/tasks"
	"github.com/gophercloud/gophercloud/v2/pagination"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/notque/openstack-mcp-server/internal/auth"
	"github.com/notque/openstack-mcp-server/internal/tools/shared"
)

// Register adds all Glance tools to the MCP server.
// When admin is true, admin-only tools (tasks) are registered.
func Register(s *mcpserver.MCPServer, provider *auth.Provider, admin bool) {
	s.AddTool(listImagesTool, listImagesHandler(provider))
	s.AddTool(getImageTool, getImageHandler(provider))
	s.AddTool(listImageMembersTool, listImageMembersHandler(provider))
	if admin {
		s.AddTool(listTasksTool, listTasksHandler(provider))
	}
}

var listImagesTool = mcp.NewTool("glance_list_images",
	mcp.WithDescription("List images available in the image service. Returns ID, name, status, visibility, disk/container format, and size."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithString("name", mcp.Description("Filter by image name")),
	mcp.WithString("status", mcp.Description("Filter by image status (queued, saving, active, killed, deleted, deactivated)")),
	mcp.WithString("visibility", mcp.Description("Filter by visibility (public, private, shared, community)")),
	mcp.WithString("owner", mcp.Description("Filter by owner project ID")),
)

var getImageTool = mcp.NewTool("glance_get_image",
	mcp.WithDescription("Get detailed information about a specific image."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithString("image_id", mcp.Required(), mcp.Description("The UUID of the image to retrieve")),
)

func listImagesHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.ImageClient()
		if err != nil {
			return shared.ToolError("failed to get image client: %v", err), nil
		}

		opts := images.ListOpts{}
		if v := shared.StringParam(request, "name"); v != "" {
			opts.Name = v
		}
		if v := shared.StringParam(request, "status"); v != "" {
			opts.Status = images.ImageStatus(v)
		}
		if v := shared.StringParam(request, "visibility"); v != "" {
			opts.Visibility = images.ImageVisibility(v)
		}
		if v := shared.StringParam(request, "owner"); v != "" {
			opts.Owner = v
		}

		var result []map[string]any
		err = images.List(client, opts).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
			allImages, err := images.ExtractImages(page)
			if err != nil {
				return false, err
			}
			for _, img := range allImages {
				result = append(result, map[string]any{
					"id":               img.ID,
					"name":             img.Name,
					"status":           img.Status,
					"visibility":       img.Visibility,
					"min_disk":         img.MinDiskGigabytes,
					"min_ram":          img.MinRAMMegabytes,
					"size":             img.SizeBytes,
					"container_format": img.ContainerFormat,
					"disk_format":      img.DiskFormat,
					"created_at":       img.CreatedAt,
				})
			}
			return true, nil
		})
		if err != nil {
			return shared.ToolError("failed to list images: %v", err), nil
		}

		out, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

func getImageHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.ImageClient()
		if err != nil {
			return shared.ToolError("failed to get image client: %v", err), nil
		}

		imageID := shared.StringParam(request, "image_id")
		if imageID == "" {
			return shared.ToolError("image_id is required"), nil
		}
		if errResult := shared.ValidateUUID(imageID, "image_id"); errResult != nil {
			return errResult, nil
		}

		img, err := images.Get(ctx, client, imageID).Extract()
		if err != nil {
			return shared.ToolError("failed to get image %s: %v", imageID, err), nil
		}

		out, err := json.MarshalIndent(img, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

// --- Image Members ---

var listImageMembersTool = mcp.NewTool("glance_list_image_members",
	mcp.WithDescription("List members an image is shared with. Returns member ID, image ID, status, created_at, and updated_at."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithString("image_id", mcp.Required(), mcp.Description("The UUID of the image to list members for")),
)

func listImageMembersHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.ImageClient()
		if err != nil {
			return shared.ToolError("failed to get image client: %v", err), nil
		}

		imageID := shared.StringParam(request, "image_id")
		if imageID == "" {
			return shared.ToolError("image_id is required"), nil
		}
		if errResult := shared.ValidateUUID(imageID, "image_id"); errResult != nil {
			return errResult, nil
		}

		result := make([]map[string]any, 0)
		err = members.List(client, imageID).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
			memberList, err := members.ExtractMembers(page)
			if err != nil {
				return false, err
			}
			for _, m := range memberList {
				result = append(result, map[string]any{
					"member_id":  m.MemberID,
					"image_id":   m.ImageID,
					"status":     m.Status,
					"created_at": m.CreatedAt,
					"updated_at": m.UpdatedAt,
				})
			}
			return true, nil
		})
		if err != nil {
			return shared.ToolError("failed to list image members for %s: %v", imageID, err), nil
		}

		out, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

// --- Tasks (Admin) ---

var listTasksTool = mcp.NewTool("glance_list_tasks",
	mcp.WithDescription("[Admin] List image import tasks. Requires admin role."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithString("status", mcp.Description("Filter by task status (pending, processing, success, failure)")),
	mcp.WithString("type", mcp.Description("Filter by task type")),
)

func listTasksHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.ImageClient()
		if err != nil {
			return shared.ToolError("failed to get image client: %v", err), nil
		}

		opts := tasks.ListOpts{}
		if v := shared.StringParam(request, "status"); v != "" {
			opts.Status = tasks.TaskStatus(v)
		}
		if v := shared.StringParam(request, "type"); v != "" {
			opts.Type = v
		}

		result := make([]map[string]any, 0)
		err = tasks.List(client, opts).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
			taskList, err := tasks.ExtractTasks(page)
			if err != nil {
				return false, err
			}
			for _, t := range taskList {
				result = append(result, map[string]any{
					"id":         t.ID,
					"type":       t.Type,
					"status":     t.Status,
					"owner":      t.Owner,
					"created_at": t.CreatedAt,
					"updated_at": t.UpdatedAt,
				})
			}
			return true, nil
		})
		if err != nil {
			return shared.ToolError("failed to list tasks: %v", err), nil
		}

		out, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}
