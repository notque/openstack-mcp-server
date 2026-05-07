// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

// Package swift provides MCP tools for OpenStack Object Storage (Swift) operations.
package swift

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/gophercloud/gophercloud/v2/openstack/objectstorage/v1/containers"
	"github.com/gophercloud/gophercloud/v2/openstack/objectstorage/v1/objects"
	"github.com/gophercloud/gophercloud/v2/pagination"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/notque/openstack-mcp-server/internal/auth"
	"github.com/notque/openstack-mcp-server/internal/tools/shared"
)

// Register adds all Swift tools to the MCP server.
// When readOnly is true, mutating tools (upload/delete objects) are not registered.
func Register(s *mcpserver.MCPServer, provider *auth.Provider, readOnly, _ bool) {
	s.AddTool(listContainersTool, listContainersHandler(provider))
	s.AddTool(listObjectsTool, listObjectsHandler(provider))
	s.AddTool(getObjectMetadataTool, getObjectMetadataHandler(provider))
	if !readOnly {
		s.AddTool(uploadObjectTool, uploadObjectHandler(provider))
		s.AddTool(deleteObjectTool, deleteObjectHandler(provider))
	}
}

var listContainersTool = mcp.NewTool("swift_list_containers",
	mcp.WithDescription("List object storage containers in the current account. Returns container name, object count, and total bytes."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithString("prefix", mcp.Description("Filter containers by name prefix")),
	mcp.WithNumber("limit", mcp.Description("Maximum number of containers to return (default 100)")),
)

var listObjectsTool = mcp.NewTool("swift_list_objects",
	mcp.WithDescription("List objects in a container. Returns object name, size in bytes, content_type, last_modified, and hash."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithString("container", mcp.Required(), mcp.Description("The name of the container to list objects from")),
	mcp.WithString("prefix", mcp.Description("Filter objects by name prefix")),
	mcp.WithString("delimiter", mcp.Description("Delimiter for pseudo-directory listings (e.g. '/')")),
	mcp.WithNumber("limit", mcp.Description("Maximum number of objects to return (default 100)")),
)

var getObjectMetadataTool = mcp.NewTool("swift_get_object_metadata",
	mcp.WithDescription("Get metadata for a specific object (not the object content). Returns content_type, content_length, etag, and last_modified."),
	mcp.WithReadOnlyHintAnnotation(true),
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

// --- Write Tools ---

var uploadObjectTool = mcp.NewTool("swift_upload_object",
	mcp.WithDescription("Upload a text object to a container. Creates or overwrites the object unless safe_write is enabled."),
	mcp.WithDestructiveHintAnnotation(true),
	mcp.WithString("container", mcp.Required(), mcp.Description("The name of the container")),
	mcp.WithString("object", mcp.Required(), mcp.Description("The name (path) of the object")),
	mcp.WithString("content", mcp.Required(), mcp.Description("The text content to upload")),
	mcp.WithString("content_type", mcp.Description("Content type (default: application/octet-stream)")),
	mcp.WithBoolean("safe_write", mcp.Description("If true, fails if object already exists (sets If-None-Match: *)")),
	mcp.WithBoolean("confirmed", mcp.Description("Set to true to execute. Without this, returns a preview of the action.")),
)

var deleteObjectTool = mcp.NewTool("swift_delete_object",
	mcp.WithDescription("Delete an object from a container."),
	mcp.WithDestructiveHintAnnotation(true),
	mcp.WithString("container", mcp.Required(), mcp.Description("The name of the container")),
	mcp.WithString("object", mcp.Required(), mcp.Description("The name (path) of the object")),
	mcp.WithBoolean("confirmed", mcp.Description("Set to true to execute. Without this, returns a preview of the action.")),
)

func uploadObjectHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.ObjectStorageClient()
		if err != nil {
			return shared.ToolError("failed to get object storage client: %v", err), nil
		}

		container := shared.StringParam(request, "container")
		if errResult := shared.ValidatePathSegment(container, "container"); errResult != nil {
			return errResult, nil
		}

		object := shared.StringParam(request, "object")
		if errResult := shared.ValidatePathSegment(object, "object"); errResult != nil {
			return errResult, nil
		}

		content := shared.StringParam(request, "content")
		if content == "" {
			return shared.ToolError("content is required"), nil
		}

		contentType := shared.StringParam(request, "content_type")
		if contentType == "" {
			contentType = "application/octet-stream"
		}

		safeWrite := shared.BoolParam(request, "safe_write")

		preview := fmt.Sprintf("Will UPLOAD object '%s' to container '%s', %d bytes, content_type: %s",
			object, container, len(content), contentType)
		if safeWrite {
			preview += " (safe mode: will fail if object already exists)"
		}

		if result := shared.RequireConfirmation(request, preview); result != nil {
			return result, nil
		}

		createOpts := objects.CreateOpts{
			Content:     bytes.NewReader([]byte(content)),
			ContentType: contentType,
		}
		if safeWrite {
			createOpts.IfNoneMatch = "*"
		}

		_, err = objects.Create(ctx, client, container, object, createOpts).Extract()
		if err != nil {
			return shared.ToolError("failed to upload object %s/%s: %v", container, object, err), nil
		}

		return shared.ToolResult(fmt.Sprintf("Successfully uploaded object '%s' to container '%s' (%d bytes)", object, container, len(content))), nil
	}
}

func deleteObjectHandler(provider *auth.Provider) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := provider.ObjectStorageClient()
		if err != nil {
			return shared.ToolError("failed to get object storage client: %v", err), nil
		}

		container := shared.StringParam(request, "container")
		if errResult := shared.ValidatePathSegment(container, "container"); errResult != nil {
			return errResult, nil
		}

		object := shared.StringParam(request, "object")
		if errResult := shared.ValidatePathSegment(object, "object"); errResult != nil {
			return errResult, nil
		}

		// Always fetch metadata to verify existence and build preview.
		header, err := objects.Get(ctx, client, container, object, nil).Extract()
		if err != nil {
			return shared.ToolError("failed to get object metadata %s/%s: %v", container, object, err), nil
		}

		preview := fmt.Sprintf("Will DELETE object '%s' from container '%s' (size: %d bytes, content_type: %s)",
			object, container, header.ContentLength, header.ContentType)
		if result := shared.RequireConfirmation(request, preview); result != nil {
			return result, nil
		}

		_, err = objects.Delete(ctx, client, container, object, nil).Extract()
		if err != nil {
			return shared.ToolError("failed to delete object %s/%s: %v", container, object, err), nil
		}

		return shared.ToolResult(fmt.Sprintf("Successfully deleted object '%s' from container '%s'", object, container)), nil
	}
}
