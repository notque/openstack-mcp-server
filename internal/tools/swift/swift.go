// Package swift provides MCP tools for OpenStack Object Storage (Swift) operations.
package swift

import (
	"context"
	"encoding/json"

	"github.com/gophercloud/gophercloud/v2/openstack/objectstorage/v1/containers"
	"github.com/gophercloud/gophercloud/v2/openstack/objectstorage/v1/objects"
	"github.com/gophercloud/gophercloud/v2/pagination"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/notque/openstack-mcp-server/internal/auth"
	"github.com/notque/openstack-mcp-server/internal/tools/shared"
)

// Register adds all Swift tools to the MCP server.
func Register(s *mcpserver.MCPServer, provider *auth.Provider) {
	s.AddTool(listContainersTool, listContainersHandler(provider))
	s.AddTool(listObjectsTool, listObjectsHandler(provider))
	s.AddTool(getObjectMetadataTool, getObjectMetadataHandler(provider))
}

var listContainersTool = mcp.NewTool("swift_list_containers",
	mcp.WithDescription("List object storage containers in the current account. Returns container name, object count, and total bytes."),
	mcp.WithString("prefix", mcp.Description("Filter containers by name prefix")),
	mcp.WithNumber("limit", mcp.Description("Maximum number of containers to return (default 100)")),
)

var listObjectsTool = mcp.NewTool("swift_list_objects",
	mcp.WithDescription("List objects in a container. Returns object name, size in bytes, content_type, last_modified, and hash."),
	mcp.WithString("container", mcp.Required(), mcp.Description("The name of the container to list objects from")),
	mcp.WithString("prefix", mcp.Description("Filter objects by name prefix")),
	mcp.WithString("delimiter", mcp.Description("Delimiter for pseudo-directory listings (e.g. '/')")),
	mcp.WithNumber("limit", mcp.Description("Maximum number of objects to return (default 100)")),
)

var getObjectMetadataTool = mcp.NewTool("swift_get_object_metadata",
	mcp.WithDescription("Get metadata for a specific object (not the object content). Returns content_type, content_length, etag, and last_modified."),
	mcp.WithString("container", mcp.Required(), mcp.Description("The name of the container")),
	mcp.WithString("object", mcp.Required(), mcp.Description("The name of the object")),
)

func listContainersHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.ObjectStorageClient()
		if err != nil {
			return shared.ToolError("failed to get object storage client: %v", err), nil
		}

		limit := int(shared.NumberParam(request, "limit"))
		if limit == 0 {
			limit = 100
		}

		opts := containers.ListOpts{
			Prefix: shared.StringParam(request, "prefix"),
			Limit:  limit,
		}

		var result []map[string]any
		err = containers.List(client, opts).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
			containerList, err := containers.ExtractInfo(page)
			if err != nil {
				return false, err
			}
			for _, c := range containerList {
				result = append(result, map[string]any{
					"name":  c.Name,
					"count": c.Count,
					"bytes": c.Bytes,
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

func listObjectsHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.ObjectStorageClient()
		if err != nil {
			return shared.ToolError("failed to get object storage client: %v", err), nil
		}

		containerName := shared.StringParam(request, "container")
		if containerName == "" {
			return shared.ToolError("container is required"), nil
		}

		limit := int(shared.NumberParam(request, "limit"))
		if limit == 0 {
			limit = 100
		}

		opts := objects.ListOpts{
			Prefix:    shared.StringParam(request, "prefix"),
			Delimiter: shared.StringParam(request, "delimiter"),
			Limit:     limit,
		}

		var result []map[string]any
		err = objects.List(client, containerName, opts).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
			objectList, err := objects.ExtractInfo(page)
			if err != nil {
				return false, err
			}
			for _, o := range objectList {
				result = append(result, map[string]any{
					"name":          o.Name,
					"bytes":         o.Bytes,
					"content_type":  o.ContentType,
					"last_modified": o.LastModified,
					"hash":          o.Hash,
				})
			}
			return true, nil
		})
		if err != nil {
			return shared.ToolError("failed to list objects: %v", err), nil
		}

		out, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}

func getObjectMetadataHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.ObjectStorageClient()
		if err != nil {
			return shared.ToolError("failed to get object storage client: %v", err), nil
		}

		containerName := shared.StringParam(request, "container")
		if containerName == "" {
			return shared.ToolError("container is required"), nil
		}

		objectName := shared.StringParam(request, "object")
		if objectName == "" {
			return shared.ToolError("object is required"), nil
		}

		header, err := objects.Get(ctx, client, containerName, objectName, nil).Extract()
		if err != nil {
			return shared.ToolError("failed to get object metadata %s/%s: %v", containerName, objectName, err), nil
		}

		metadata := map[string]any{
			"content_type":   header.ContentType,
			"content_length": header.ContentLength,
			"etag":           header.ETag,
			"last_modified":  header.LastModified,
		}

		out, err := json.MarshalIndent(metadata, "", "  ")
		if err != nil {
			return shared.ToolError("failed to marshal response: %v", err), nil
		}
		return shared.ToolResult(string(out)), nil
	}
}
