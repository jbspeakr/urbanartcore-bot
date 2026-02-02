package infrastructure

import (
	"log/slog"
	"testing"
)

func TestSanitizeSecrets(t *testing.T) {
	tests := []struct {
		name       string
		attr       slog.Attr
		expected   string
		isRedacted bool
	}{
		{
			name:       "password field is redacted",
			attr:       slog.String("password", "secret123"),
			expected:   "sec***123",
			isRedacted: true,
		},
		{
			name:       "api_key field is redacted",
			attr:       slog.String("api_key", "sk-1234567890abcdef"),
			expected:   "sk-***def",
			isRedacted: true,
		},
		{
			name:       "short secret is fully redacted",
			attr:       slog.String("token", "abc"),
			expected:   "***",
			isRedacted: true,
		},
		{
			name:       "UAC specific key is redacted",
			attr:       slog.String("uac_mistral_api_key", "sk-proj-abcdef123456"),
			expected:   "sk-***456",
			isRedacted: true,
		},
		{
			name:       "normal field is not redacted",
			attr:       slog.String("username", "user123"),
			expected:   "user123",
			isRedacted: false,
		},
		{
			name:       "int field with sensitive name is redacted",
			attr:       slog.Int("secret_count", 42),
			expected:   "[REDACTED]",
			isRedacted: true,
		},
		{
			name:       "empty secret is handled",
			attr:       slog.String("password", ""),
			expected:   "",
			isRedacted: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeSecrets(nil, tt.attr)

			if tt.isRedacted {
				if result.Value.String() == tt.attr.Value.String() {
					t.Errorf("Expected value to be redacted, but it wasn't changed")
				}
				if result.Value.String() != tt.expected {
					t.Errorf("Expected redacted value %q, got %q", tt.expected, result.Value.String())
				}
			} else {
				if result.Value.String() != tt.expected {
					t.Errorf("Expected value %q, got %q", tt.expected, result.Value.String())
				}
			}
		})
	}
}
