package shared

import (
	"testing"
)

func TestSanitizeResponse_RedactsFernetToken(t *testing.T) {
	// Fernet tokens start with gAAAAA and are 180+ chars
	input := `{"token": "gAAAAA` + string(make([]byte, 150)) + `"}`
	// Replace null bytes with valid chars for the test
	input = `{"info": "gAAAAAbcdefghijklmnopqrstuvwxyz0123456789_-ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_-abcdefghijk"}`

	result := SanitizeResponse(input)
	if result == input {
		t.Error("expected fernet token to be redacted")
	}
	if !contains(result, "[REDACTED_TOKEN]") {
		t.Errorf("expected [REDACTED_TOKEN] in output, got: %s", result)
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
			name:  "x-auth-token header",
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

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
