// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

package shared

import (
	"testing"
)

func TestValidateUUID_ValidUUIDs(t *testing.T) {
	validUUIDs := []string{
		"550e8400-e29b-41d4-a716-446655440000",
		"6ba7b810-9dad-11d1-80b4-00c04fd430c8",
		"00000000-0000-0000-0000-000000000000",
		"AAAAAAAA-BBBB-CCCC-DDDD-EEEEEEEEEEEE",
		// OpenStack Keystone returns UUIDs without hyphens
		"a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6",
		"f6e5d4c3b2a1f6e5d4c3b2a1f6e5d4c3",
	}
	for _, uuid := range validUUIDs {
		if result := ValidateUUID(uuid, "test_id"); result != nil {
			t.Errorf("ValidateUUID(%q) should pass, got error", uuid)
		}
	}
}

func TestValidateUUID_InvalidUUIDs(t *testing.T) {
	invalidUUIDs := []struct {
		name  string
		value string
	}{
		{"path traversal", "../../admin/tokens"},
		{"too short", "abc123"},
		{"not hex", "zzzzzzzz-zzzz-zzzz-zzzz-zzzzzzzzzzzz"},
		{"sql injection", "'; DROP TABLE servers; --"},
		{"query injection", "abc&admin=true"},
		{"empty", ""},
		{"with spaces", "550e8400 e29b 41d4 a716 446655440000"},
		{"newlines", "550e8400-e29b-41d4-a716\n-446655440000"},
	}
	for _, tt := range invalidUUIDs {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateUUID(tt.value, "test_id")
			if result == nil {
				t.Errorf("ValidateUUID(%q) should fail", tt.value)
			}
			if result != nil && !result.IsError {
				t.Errorf("ValidateUUID(%q) should return IsError=true", tt.value)
			}
		})
	}
}

func TestValidatePathSegment_Valid(t *testing.T) {
	validSegments := []string{
		"my-account",
		"repo-name",
		"my_image/nested",
		"account.name",
		"simple123",
	}
	for _, seg := range validSegments {
		if result := ValidatePathSegment(seg, "test"); result != nil {
			t.Errorf("ValidatePathSegment(%q) should pass, got error", seg)
		}
	}
}

func TestValidatePathSegment_Invalid(t *testing.T) {
	invalidSegments := []struct {
		name  string
		value string
	}{
		{"path traversal", "../../../etc/passwd"},
		{"double dot prefix", "..secret"},
		{"query string", "account?admin=true"},
		{"fragment", "account#fragment"},
		{"empty", ""},
		{"control chars", "account\x00name"},
		{"space", "my account"},
		{"starts with dot", ".hidden"},
		{"starts with dash", "-invalid"},
	}
	for _, tt := range invalidSegments {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidatePathSegment(tt.value, "test")
			if result == nil {
				t.Errorf("ValidatePathSegment(%q) should fail", tt.value)
			}
		})
	}
}

func TestSafeQueryParams_EncodesValues(t *testing.T) {
	tests := []struct {
		name   string
		params map[string]string
		want   string
	}{
		{
			name:   "single param",
			params: map[string]string{"status": "active"},
			want:   "?status=active",
		},
		{
			name:   "empty value skipped",
			params: map[string]string{"status": "", "name": "test"},
			want:   "?name=test",
		},
		{
			name:   "all empty",
			params: map[string]string{"status": "", "name": ""},
			want:   "",
		},
		{
			name:   "special chars encoded",
			params: map[string]string{"query": "foo&bar=baz"},
			want:   "?query=foo%26bar%3Dbaz",
		},
		{
			name:   "spaces encoded",
			params: map[string]string{"name": "my server"},
			want:   "?name=my+server",
		},
		{
			name:   "multiple params joined",
			params: map[string]string{"action": "create", "outcome": "success"},
			want:   "?action=create&outcome=success",
		},
		{
			name:   "path traversal in value encoded",
			params: map[string]string{"name": "../../etc/passwd"},
			want:   "?name=..%2F..%2Fetc%2Fpasswd",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SafeQueryParams(tt.params)
			if got != tt.want {
				t.Errorf("SafeQueryParams() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSanitizeResponse_RedactsAdminPass(t *testing.T) {
	input := `{"server": {"id": "abc", "adminPass": "MySecretPassword123!", "name": "test"}}`
	result := SanitizeResponse(input)
	if result == input {
		t.Errorf("SanitizeResponse should redact adminPass field")
	}
	if !containsAny(result, `"adminPass": "[REDACTED]"`) {
		t.Errorf("expected adminPass to be redacted, got: %s", result)
	}
	// Verify other fields preserved
	if !containsAny(result, `"name": "test"`) {
		t.Errorf("expected name field to be preserved, got: %s", result)
	}
}

func TestSanitizeResponse_RedactsAdminPassCaseInsensitive(t *testing.T) {
	input := `{"AdminPass": "secret123"}`
	result := SanitizeResponse(input)
	if containsAny(result, "secret123") {
		t.Errorf("SanitizeResponse should redact AdminPass (case insensitive), got: %s", result)
	}
}

func containsAny(s, substr string) bool {
	return s != "" && substr != "" && contains(s, substr)
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
