// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

package shared

import (
	"strings"
	"testing"
)

func TestSanitizeResponse_RedactsFernetToken(t *testing.T) {
	// Fernet tokens start with gAAAAA and are 100+ chars of base64url
	token := "gAAAAAbcdefghijklmnopqrstuvwxyz0123456789_-ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_-abcdefghijk"
	input := `{"info": "` + token + `"}`

	result := SanitizeResponse(input)
	if strings.Contains(result, "gAAAAA") {
		t.Errorf("expected fernet token to be redacted, got: %s", result)
	}
	if !strings.Contains(result, "[REDACTED_TOKEN]") {
		t.Errorf("expected [REDACTED_TOKEN] in output, got: %s", result)
	}
}

func TestSanitizeResponse_RedactsFernetWithPadding(t *testing.T) {
	// Real fernet tokens end with base64 padding (=)
	token := "gAAAAAbcdefghijklmnopqrstuvwxyz0123456789_-ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_-abcdef=="
	input := `token is ` + token + ` here`

	result := SanitizeResponse(input)
	if strings.Contains(result, "gAAAAA") {
		t.Errorf("expected fernet token with padding to be redacted, got: %s", result)
	}
}

func TestSanitizeResponse_RedactsSensitiveJSONKeys(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "password field",
			input: `{"username": "admin", "password": "supersecret123"}`,
			want:  `{"username": "admin", "password": "[REDACTED]"}`,
		},
		{
			name:  "auth_token field",
			input: `{"auth_token": "abc123def456", "status": "ok"}`,
			want:  `{"auth_token": "[REDACTED]", "status": "ok"}`,
		},
		{
			name:  "x-auth-token header in JSON",
			input: `{"x-auth-token": "mytoken123", "content-type": "application/json"}`,
			want:  `{"x-auth-token": "[REDACTED]", "content-type": "application/json"}`,
		},
		{
			name:  "secret field",
			input: `{"secret": "verysecret", "name": "myapp"}`,
			want:  `{"secret": "[REDACTED]", "name": "myapp"}`,
		},
		{
			name:  "case insensitive",
			input: `{"Password": "hunter2", "name": "test"}`,
			want:  `{"Password": "[REDACTED]", "name": "test"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeResponse(tt.input)
			if got != tt.want {
				t.Errorf("SanitizeResponse() =\n  %s\nwant:\n  %s", got, tt.want)
			}
		})
	}
}

func TestSanitizeResponse_HandlesEscapedQuotesInValues(t *testing.T) {
	// A password containing escaped quotes: pass"word
	// In JSON: "password": "pass\"word"
	input := `{"password": "pass\"word", "name": "test"}`
	result := SanitizeResponse(input)
	// The password value (including escaped quote) should be fully redacted
	if strings.Contains(result, "pass\\\"word") || strings.Contains(result, `pass"word`) {
		t.Errorf("expected escaped-quote password to be fully redacted, got: %s", result)
	}
	if !strings.Contains(result, `"password": "[REDACTED]"`) {
		t.Errorf("expected redacted password key, got: %s", result)
	}
	// Normal fields should be preserved
	if !strings.Contains(result, `"name": "test"`) {
		t.Errorf("expected name field to be preserved, got: %s", result)
	}
}

func TestSanitizeResponse_RedactsHTTPAuthHeaders(t *testing.T) {
	// gophercloud error messages can contain HTTP headers
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "X-Auth-Token header",
			input: `Response headers: X-Auth-Token: gAAAAAbcdef123456`,
		},
		{
			name:  "X-Subject-Token header",
			input: `X-Subject-Token: some-long-token-value-here`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeResponse(tt.input)
			if strings.Contains(result, "gAAAAA") || strings.Contains(result, "some-long-token") {
				t.Errorf("expected HTTP auth header to be redacted, got: %s", result)
			}
			if !strings.Contains(result, "[REDACTED_TOKEN]") {
				t.Errorf("expected [REDACTED_TOKEN] in output, got: %s", result)
			}
		})
	}
}

func TestSanitizeResponse_PositiveTokenAssertion(t *testing.T) {
	// Set a known token and verify it gets redacted regardless of format
	testToken := "this-is-a-real-auth-token-that-should-be-caught-always"
	SetCurrentToken(testToken)
	defer SetCurrentToken("") // cleanup

	// Token in a random context (not JSON, not header format)
	input := `Error: request failed with token this-is-a-real-auth-token-that-should-be-caught-always in context`
	result := SanitizeResponse(input)
	if strings.Contains(result, testToken) {
		t.Errorf("positive assertion should catch token regardless of format, got: %s", result)
	}
}

func TestSanitizeResponse_PreservesNormalContent(t *testing.T) {
	normal := `{"id": "abc-123", "name": "my-server", "status": "ACTIVE", "addresses": {"network": [{"addr": "10.0.0.1"}]}}`
	result := SanitizeResponse(normal)
	if result != normal {
		t.Errorf("SanitizeResponse modified normal content:\n  got:  %s\n  want: %s", result, normal)
	}
}

func TestSanitizeResponse_HandlesEmptyString(t *testing.T) {
	result := SanitizeResponse("")
	if result != "" {
		t.Errorf("expected empty string, got: %s", result)
	}
}

func TestSanitizeResponse_GophercloudErrorFormat(t *testing.T) {
	// Simulate a gophercloud ErrUnexpectedResponseCode error string
	input := `Expected HTTP response code [200] when accessing [GET https://identity-3.qa-de-1.cloud.sap/v3/auth/tokens], but got 401 instead: {"error":{"message":"The request you have made requires authentication.","code":401}}`
	result := SanitizeResponse(input)
	// This should pass through mostly unchanged (no secrets in this format)
	if result != input {
		t.Errorf("should not modify standard gophercloud error without secrets, got: %s", result)
	}
}

func TestRedactToken_OnlyRedactsToken(t *testing.T) {
	// RedactToken should ONLY strip the auth token, leaving other secrets intact.
	// This is critical for ToolResultRaw (app credential creation) where the
	// secret IS the intended output but the auth token must never leak.
	testToken := "gAAAAA-this-is-the-real-auth-token-that-must-be-redacted-always-no-matter-what"
	SetCurrentToken(testToken)
	defer SetCurrentToken("")

	// Input intentionally contains both the auth token AND an app credential secret
	input := `{"secret": "user-app-cred-secret-value", "token_appeared": "` + testToken + `"}`
	result := RedactToken(input)

	// Auth token must be gone
	if strings.Contains(result, testToken) {
		t.Errorf("RedactToken should redact the auth token, got: %s", result)
	}
	// App credential secret must be preserved (this is the whole point of RedactToken vs SanitizeResponse)
	if !strings.Contains(result, "user-app-cred-secret-value") {
		t.Errorf("RedactToken should preserve non-token secrets, got: %s", result)
	}
}

func TestRedactToken_IgnoresShortTokens(t *testing.T) {
	// Tokens under 20 chars are skipped to avoid false positives on common strings
	SetCurrentToken("short")
	defer SetCurrentToken("")

	input := `The word short appears here`
	result := RedactToken(input)
	if result != input {
		t.Errorf("RedactToken should not redact short tokens, got: %s", result)
	}
}

func TestRedactToken_EmptyToken(t *testing.T) {
	SetCurrentToken("")
	input := `{"data": "normal content"}`
	result := RedactToken(input)
	if result != input {
		t.Errorf("RedactToken with empty token should be no-op, got: %s", result)
	}
}
