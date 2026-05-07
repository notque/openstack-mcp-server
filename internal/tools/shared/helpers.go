// Package shared provides common helpers for MCP tool implementations.
package shared

import (
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

// ToolError creates an MCP tool error response.
func ToolError(msg string, args ...any) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{
			mcp.NewTextContent(fmt.Sprintf(msg, args...)),
		},
	}
}

// ToolResult creates an MCP tool success response.
func ToolResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.NewTextContent(text),
		},
	}
}

// StringParam extracts a string parameter from an MCP request.
func StringParam(req mcp.CallToolRequest, key string) string {
	args := req.GetArguments()
	if args == nil {
		return ""
	}
	if v, ok := args[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// NumberParam extracts a numeric parameter from an MCP request.
func NumberParam(req mcp.CallToolRequest, key string) float64 {
	args := req.GetArguments()
	if args == nil {
		return 0
	}
	if v, ok := args[key]; ok {
		if n, ok := v.(float64); ok {
			return n
		}
	}
	return 0
}
