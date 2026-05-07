// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

// Package shared provides common helpers for MCP tool implementations.
package shared

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

// uuidPattern validates that a string is a proper UUID format.
// Accepts both hyphenated (8-4-4-4-12) and non-hyphenated (32 hex chars) formats,
// because OpenStack Keystone commonly returns IDs without hyphens.
// Used to prevent path traversal attacks via ID parameters in URL construction.
var uuidPattern = regexp.MustCompile(`(?i)^([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}|[0-9a-f]{32})$`)

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
	if strings.Contains(value, "..") {
		return ToolError("%s must not contain path traversal sequences (got: %q)", paramName, value)
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

// BoolParam extracts a boolean parameter from an MCP request.
func BoolParam(req mcp.CallToolRequest, key string) bool {
	args := req.GetArguments()
	if args == nil {
		return false
	}
	if v, ok := args[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

// RequireConfirmation checks the "confirmed" parameter in a tool request.
// If confirmed=false (or absent), returns a preview message asking the caller
// to re-invoke with confirmed=true. Returns nil when confirmed, meaning the
// handler should proceed with execution.
//
// Usage pattern in write handlers:
//
//	preview := fmt.Sprintf("Will DELETE volume %q (%s, %dGiB)", name, id, size)
//	if result := shared.RequireConfirmation(request, preview); result != nil {
//	    return result, nil
//	}
//	// proceed with actual deletion...
func RequireConfirmation(req mcp.CallToolRequest, preview string) *mcp.CallToolResult {
	if BoolParam(req, "confirmed") {
		return nil
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.NewTextContent(fmt.Sprintf(
				"CONFIRMATION REQUIRED\n\n%s\n\nTo proceed, re-call this tool with confirmed=true.",
				SanitizeResponse(preview),
			)),
		},
	}
}
