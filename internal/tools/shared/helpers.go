// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

// Package shared provides common helpers for MCP tool implementations.
package shared

import (
	"fmt"
	"net/url"
	"regexp"

	"github.com/mark3labs/mcp-go/mcp"
)

// uuidPattern validates that a string is a proper UUID format.
// Used to prevent path traversal attacks via ID parameters in URL construction.
var uuidPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// safePathSegmentPattern validates that a string is a safe URL path segment.
// Allows alphanumeric, hyphens, underscores, dots, and forward slashes (for repo paths).
// Rejects: .., control chars, query strings, fragments.
var safePathSegmentPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._/\-]*$`)

// ValidateUUID checks if a string is a valid UUID. Returns an error tool result if invalid.
func ValidateUUID(value, paramName string) *mcp.CallToolResult {
	if !uuidPattern.MatchString(value) {
		return ToolError("%s must be a valid UUID (got: %q)", paramName, value)
	}
	return nil
}

// ValidatePathSegment checks if a string is safe for use in a URL path.
// Rejects empty strings, path traversal attempts (..), and control characters.
func ValidatePathSegment(value, paramName string) *mcp.CallToolResult {
	if value == "" {
		return ToolError("%s is required", paramName)
	}
	if !safePathSegmentPattern.MatchString(value) {
		return ToolError("%s contains invalid characters (got: %q)", paramName, value)
	}
	return nil
}

// SafeQueryParams builds a URL query string from key-value pairs, properly encoding values.
func SafeQueryParams(params map[string]string) string {
	values := url.Values{}
	for k, v := range params {
		if v != "" {
			values.Set(k, v)
		}
	}
	if encoded := values.Encode(); encoded != "" {
		return "?" + encoded
	}
	return ""
}

// ToolError creates an MCP tool error response.
// Error messages are sanitized to prevent accidental credential leakage
// (e.g., from gophercloud error strings that may include response bodies).
func ToolError(msg string, args ...any) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{
			mcp.NewTextContent(SanitizeResponse(fmt.Sprintf(msg, args...))),
		},
	}
}

// ToolResult creates an MCP tool success response.
// All responses are sanitized to prevent accidental credential leakage.
func ToolResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.NewTextContent(SanitizeResponse(text)),
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

// ToolResultRaw creates an MCP tool success response WITHOUT full sanitization.
// USE WITH EXTREME CARE. Only for responses that intentionally contain
// sensitive material the user explicitly requested (e.g., a newly created
// application credential secret that is only visible at creation time).
//
// SECURITY: Even "raw" responses still redact the current auth token via
// RedactToken(). The auth token must NEVER reach the LLM regardless of context.
func ToolResultRaw(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.NewTextContent(RedactToken(text)),
		},
	}
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
