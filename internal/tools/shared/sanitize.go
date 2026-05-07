// Package shared provides common helpers for MCP tool implementations.
package shared

import (
	"regexp"
	"strings"
	"sync"
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

// Pre-compiled patterns for sensitive JSON keys. Compiled once at init time
// to avoid per-call regex compilation overhead and ensure invalid patterns
// are caught at startup.
var sensitiveKeyPatterns []*regexp.Regexp

func init() {
	for _, key := range sensitiveKeys {
		// Match "key": "value" including values with escaped quotes
		sensitiveKeyPatterns = append(sensitiveKeyPatterns,
			regexp.MustCompile(`(?i)"`+regexp.QuoteMeta(key)+`"\s*:\s*"[^"\\]*(?:\\.[^"\\]*)*"`))
	}
}

// fernetTokenPattern matches Keystone fernet-style tokens (gAAAAA... with base64url + padding).
var fernetTokenPattern = regexp.MustCompile(`gAAAAA[A-Za-z0-9_\-=]{100,}`)

// httpAuthHeaderPattern matches auth tokens in HTTP header format (from error messages).
var httpAuthHeaderPattern = regexp.MustCompile(`(?i)(X-(?:Auth|Subject)-Token)\s*:\s*\S+`)

// currentToken holds the active auth token for positive assertion checks.
// Protected by currentTokenMu for concurrent access (SSE mode + reauth).
var (
	currentTokenMu sync.RWMutex
	currentToken   string
)

// SetCurrentToken stores the current auth token for positive assertion sanitization.
// This allows the sanitizer to check if the exact token appears in any response.
// Safe for concurrent use — called at startup and on token reauth.
func SetCurrentToken(token string) {
	currentTokenMu.Lock()
	currentToken = token
	currentTokenMu.Unlock()
}

// getCurrentToken returns the current token safely for concurrent reads.
func getCurrentToken() string {
	currentTokenMu.RLock()
	defer currentTokenMu.RUnlock()
	return currentToken
}

// SanitizeResponse scrubs known sensitive patterns from a tool response string.
// This is a defense-in-depth measure — our tools should never include secrets,
// but this catches accidental leakage from raw API responses or error messages.
//
// Three layers of protection:
// 1. Positive assertion: exact substring match for the current auth token
// 2. Pattern matching: fernet tokens, sensitive JSON keys, HTTP auth headers
// 3. Structural: pre-compiled regexes handle escaped JSON values
func SanitizeResponse(response string) string {
	// Layer 1: Positive assertion — if the actual current token appears, redact it.
	// This catches ANY format the token might appear in (no regex needed).
	if tok := getCurrentToken(); tok != "" && len(tok) > 20 && strings.Contains(response, tok) {
		response = strings.ReplaceAll(response, tok, "[REDACTED_TOKEN]")
	}

	// Layer 2a: Redact fernet-style tokens (gAAAAA... pattern)
	response = fernetTokenPattern.ReplaceAllString(response, "[REDACTED_TOKEN]")

	// Layer 2b: Redact auth tokens in HTTP header format (from gophercloud errors)
	response = httpAuthHeaderPattern.ReplaceAllString(response, "${1}: [REDACTED_TOKEN]")

	// Layer 2c: Redact sensitive JSON key-value pairs (pre-compiled, handles escaped quotes)
	for _, pattern := range sensitiveKeyPatterns {
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

// RedactToken performs ONLY the token positive-assertion check (Layer 1).
// Used by ToolResultRaw to ensure the auth token never leaks even in
// responses that intentionally contain other secrets (e.g., app credential creation).
func RedactToken(response string) string {
	if tok := getCurrentToken(); tok != "" && len(tok) > 20 && strings.Contains(response, tok) {
		response = strings.ReplaceAll(response, tok, "[REDACTED_TOKEN]")
	}
	return response
}
