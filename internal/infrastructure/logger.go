package infrastructure

import (
	"log/slog"
	"strings"
)

// SanitizeSecrets replaces sensitive information in log attributes.
// Used as the ReplaceAttr hook when setting up the logger.
func SanitizeSecrets(groups []string, attr slog.Attr) slog.Attr {
	key := strings.ToLower(attr.Key)

	// List of sensitive keys that should be redacted
	sensitiveKeys := []string{
		"password", "token", "key", "secret", "credential",
		"api_key", "access_token", "refresh_token", "bearer",
		"authorization", "auth", "mistral_key", "bluesky_password",
		"mastodon_token", "uac_mistral_api_key", "uac_bluesky_password",
		"uac_mastodon_access_token",
	}

	// Check if this attribute contains sensitive information
	for _, sensitiveKey := range sensitiveKeys {
		if strings.Contains(key, sensitiveKey) {
			// Redact the value but preserve the type and some context
			if attr.Value.Kind() == slog.KindString {
				value := attr.Value.String()
				if len(value) > 8 {
					// Show first 3 and last 3 characters with asterisks in between
					redacted := value[:3] + "***" + value[len(value)-3:]
					return slog.Attr{Key: attr.Key, Value: slog.StringValue(redacted)}
				} else if len(value) > 0 {
					// For short values, just show asterisks
					return slog.Attr{Key: attr.Key, Value: slog.StringValue("***")}
				} else {
					// For empty values, keep empty but mark as redacted field
					return slog.Attr{Key: attr.Key, Value: slog.StringValue("")}
				}
			}
			// For non-string values, replace with a placeholder
			return slog.Attr{Key: attr.Key, Value: slog.StringValue("[REDACTED]")}
		}
	}

	return attr
}
