package infrastructure

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
)

// LogLevel represents the severity of a log message.
type LogLevel string

const (
	LevelDebug LogLevel = "debug"
	LevelInfo  LogLevel = "info"
	LevelWarn  LogLevel = "warn"
	LevelError LogLevel = "error"
)

// String returns the string representation of the log level.
func (l LogLevel) String() string {
	return string(l)
}

// ToSlogLevel converts LogLevel to slog.Level.
func (l LogLevel) ToSlogLevel() slog.Level {
	switch l {
	case LevelDebug:
		return slog.LevelDebug
	case LevelInfo:
		return slog.LevelInfo
	case LevelWarn:
		return slog.LevelWarn
	case LevelError:
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// LoggerConfig holds configuration for the structured logger.
type LoggerConfig struct {
	Level  LogLevel `json:"level"`
	Format string   `json:"format"` // "json" or "text"
	Output string   `json:"output"` // "stdout", "stderr", or file path
}

// DefaultLoggerConfig returns sensible defaults for logging.
func DefaultLoggerConfig() *LoggerConfig {
	return &LoggerConfig{
		Level:  LevelInfo,
		Format: "json",
		Output: "stdout",
	}
}

// StructuredLogger wraps slog.Logger with domain-specific functionality
// and secret sanitization for the UAC Socializer application.
type StructuredLogger struct {
	logger *slog.Logger
	config *LoggerConfig
}

// NewStructuredLogger creates a new logger with the given configuration.
func NewStructuredLogger(config *LoggerConfig) (*StructuredLogger, error) {
	if config == nil {
		config = DefaultLoggerConfig()
	}

	// Determine output destination
	var writer io.Writer
	switch config.Output {
	case "stdout", "":
		writer = os.Stdout
	case "stderr":
		writer = os.Stderr
	default:
		// Assume it's a file path
		file, err := os.OpenFile(config.Output, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return nil, fmt.Errorf("failed to open log file %s: %w", config.Output, err)
		}
		writer = file
	}

	// Create handler based on format
	var handler slog.Handler
	opts := &slog.HandlerOptions{
		Level: config.Level.ToSlogLevel(),
		ReplaceAttr: sanitizeSecrets,
	}

	switch config.Format {
	case "json":
		handler = slog.NewJSONHandler(writer, opts)
	case "text":
		handler = slog.NewTextHandler(writer, opts)
	default:
		return nil, fmt.Errorf("unsupported log format: %s", config.Format)
	}

	logger := slog.New(handler)

	return &StructuredLogger{
		logger: logger,
		config: config,
	}, nil
}

// NewLoggerFromEnv creates a logger configured from environment variables.
func NewLoggerFromEnv() (*StructuredLogger, error) {
	config := DefaultLoggerConfig()

	// Read log level from environment
	if envLevel := os.Getenv("UAC_LOG_LEVEL"); envLevel != "" {
		config.Level = LogLevel(strings.ToLower(envLevel))
	}

	// Read log format from environment
	if envFormat := os.Getenv("UAC_LOG_FORMAT"); envFormat != "" {
		config.Format = strings.ToLower(envFormat)
	}

	// Read log output from environment
	if envOutput := os.Getenv("UAC_LOG_OUTPUT"); envOutput != "" {
		config.Output = envOutput
	}

	return NewStructuredLogger(config)
}

// sanitizeSecrets replaces sensitive information in log attributes.
func sanitizeSecrets(groups []string, attr slog.Attr) slog.Attr {
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

// Debug logs a debug message with optional structured fields.
func (sl *StructuredLogger) Debug(msg string, fields ...slog.Attr) {
	sl.logger.LogAttrs(context.Background(), slog.LevelDebug, msg, fields...)
}

// Info logs an info message with optional structured fields.
func (sl *StructuredLogger) Info(msg string, fields ...slog.Attr) {
	sl.logger.LogAttrs(context.Background(), slog.LevelInfo, msg, fields...)
}

// Warn logs a warning message with optional structured fields.
func (sl *StructuredLogger) Warn(msg string, fields ...slog.Attr) {
	sl.logger.LogAttrs(context.Background(), slog.LevelWarn, msg, fields...)
}

// Error logs an error message with optional structured fields.
func (sl *StructuredLogger) Error(msg string, fields ...slog.Attr) {
	sl.logger.LogAttrs(context.Background(), slog.LevelError, msg, fields...)
}

// With returns a new logger with additional context fields.
func (sl *StructuredLogger) With(fields ...slog.Attr) *StructuredLogger {
	return &StructuredLogger{
		logger: sl.logger.With(toAnySlice(fields)...),
		config: sl.config,
	}
}

// WithContext returns a new logger that includes context values.
func (sl *StructuredLogger) WithContext(ctx context.Context) *StructuredLogger {
	// Extract common context values
	var fields []slog.Attr

	if deadline, ok := ctx.Deadline(); ok {
		fields = append(fields, slog.Time("deadline", deadline))
	}

	return &StructuredLogger{
		logger: sl.logger.With(toAnySlice(fields)...),
		config: sl.config,
	}
}

// Helper function to convert []slog.Attr to []any for slog.Logger.With
func toAnySlice(attrs []slog.Attr) []any {
	result := make([]any, len(attrs)*2)
	for i, attr := range attrs {
		result[i*2] = attr.Key
		result[i*2+1] = attr.Value
	}
	return result
}

// GetLogger returns the underlying slog.Logger for advanced usage.
func (sl *StructuredLogger) GetLogger() *slog.Logger {
	return sl.logger
}