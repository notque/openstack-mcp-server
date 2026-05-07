// Package shared provides common helpers for MCP tool implementations.
package shared

import (
	"regexp"
	"strings"
)

// sensitiveKeys lists JSON keys whose values should be redacted from tool responses.
// These keys could appear in OpenStack API responses if raw responses are forwarded.
var sensitiveKeys = []string{
	"password",
	"secret",
	"token_id",
	"x-auth-token",
	"x-subject-token",
	"auth_token",
	"api_key",
	"apikey",
	"access_key",
	"secret_key",
	"private_key",
}

// tokenPattern matches Keystone-style tokens (hex or fernet-encoded).
// Keystone tokens are typically 32-char hex (UUID) or longer fernet tokens.
var tokenPattern = regexp.MustCompile(`\b(gAAAAA[A-Za-z0-9_-]{100,})\b`)

// SanitizeResponse scrubs known sensitive patterns from a tool response string.
// This is a defense-in-depth measure — our tools should never include secrets,
// but this catches accidental leakage from raw API responses or error messages.
func SanitizeResponse(response string) string {
	// Redact fernet-style tokens (used by Keystone)
	response = tokenPattern.ReplaceAllString(response, "[REDACTED_TOKEN]")

	// Redact any lines containing sensitive key patterns in JSON-like output
	for _, key := range sensitiveKeys {
		// Match "key": "value" patterns (case-insensitive key matching)
		pattern := regexp.MustCompile(`(?i)"` + regexp.QuoteMeta(key) + `"\s*:\s*"[^"]*"`)
		response = pattern.ReplaceAllStringFunc(response, func(match string) string {
			// Preserve the key but redact the value
			parts := strings.SplitN(match, ":", 2)
			if len(parts) == 2 {
				return parts[0] + `: "[REDACTED]"`
			}
			return match
		})
	}

	return response
}
