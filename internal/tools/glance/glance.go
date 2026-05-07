// Package glance provides MCP tools for OpenStack Image Service (Glance) operations.
package glance

import (
	"context"
	"encoding/json"

	"github.com/gophercloud/gophercloud/v2/openstack/image/v2/images"
	"github.com/gophercloud/gophercloud/v2/pagination"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/notque/openstack-mcp-server/internal/auth"
	"github.com/notque/openstack-mcp-server/internal/tools/shared"
)

// Register adds all Glance tools to the MCP server.
func Register(s *mcpserver.MCPServer, provider *auth.Provider) {
	s.AddTool(listImagesTool, listImagesHandler(provider))
	s.AddTool(getImageTool, getImageHandler(provider))
}

var listImagesTool = mcp.NewTool("glance_list_images",
	mcp.WithDescription("List images available in the image service. Returns ID, name, status, visibility, disk/container format, and size."),
	mcp.WithString("name", mcp.Description("Filter by image name")),
	mcp.WithString("status", mcp.Description("Filter by image status (queued, saving, active, killed, deleted, deactivated)")),
	mcp.WithString("visibility", mcp.Description("Filter by visibility (public, private, shared, community)")),
	mcp.WithString("owner", mcp.Description("Filter by owner project ID")),
)

var getImageTool = mcp.NewTool("glance_get_image",
	mcp.WithDescription("Get detailed information about a specific image."),
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
