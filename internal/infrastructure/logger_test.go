package infrastructure

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestLogLevel_String(t *testing.T) {
	tests := []struct {
		level    LogLevel
		expected string
	}{
		{LevelDebug, "debug"},
		{LevelInfo, "info"},
		{LevelWarn, "warn"},
		{LevelError, "error"},
	}

	for _, tt := range tests {
		t.Run(string(tt.level), func(t *testing.T) {
			if got := tt.level.String(); got != tt.expected {
				t.Errorf("LogLevel.String() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestLogLevel_ToSlogLevel(t *testing.T) {
	tests := []struct {
		level    LogLevel
		expected slog.Level
	}{
		{LevelDebug, slog.LevelDebug},
		{LevelInfo, slog.LevelInfo},
		{LevelWarn, slog.LevelWarn},
		{LevelError, slog.LevelError},
		{LogLevel("invalid"), slog.LevelInfo}, // Default fallback
	}

	for _, tt := range tests {
		t.Run(string(tt.level), func(t *testing.T) {
			if got := tt.level.ToSlogLevel(); got != tt.expected {
				t.Errorf("LogLevel.ToSlogLevel() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestDefaultLoggerConfig(t *testing.T) {
	config := DefaultLoggerConfig()

	if config.Level != LevelInfo {
		t.Errorf("Expected default level to be %v, got %v", LevelInfo, config.Level)
	}

	if config.Format != "json" {
		t.Errorf("Expected default format to be 'json', got %v", config.Format)
	}
}

func TestNewStructuredLogger(t *testing.T) {
	tests := []struct {
		name    string
		config  *LoggerConfig
		wantErr bool
	}{
		{
			name:    "nil config uses defaults",
			config:  nil,
			wantErr: false,
		},
		{
			name: "valid json config",
			config: &LoggerConfig{
				Level:  LevelDebug,
				Format: "json",
			},
			wantErr: false,
		},
		{
			name: "valid text config",
			config: &LoggerConfig{
				Level:  LevelInfo,
				Format: "text",
			},
			wantErr: false,
		},
		{
			name: "invalid format",
			config: &LoggerConfig{
				Level:  LevelInfo,
				Format: "invalid",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger, err := NewStructuredLogger(tt.config)

			if tt.wantErr {
				if err == nil {
					t.Errorf("NewStructuredLogger() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("NewStructuredLogger() unexpected error: %v", err)
				return
			}

			if logger == nil {
				t.Errorf("NewStructuredLogger() returned nil logger")
			}
		})
	}
}

func TestStructuredLogger_LoggingMethods(t *testing.T) {
	// Create a buffer to capture log output
	var buf bytes.Buffer

	config := &LoggerConfig{
		Level:  LevelDebug,
		Format: "json",
	}

	// Create logger with custom handler that writes to our buffer
	opts := &slog.HandlerOptions{
		Level:       config.Level.ToSlogLevel(),
		ReplaceAttr: sanitizeSecrets,
	}
	handler := slog.NewJSONHandler(&buf, opts)
	logger := &StructuredLogger{
		logger: slog.New(handler),
		config: config,
	}

	tests := []struct {
		name     string
		logFunc  func()
		level    string
		message  string
		hasField bool
		field    string
	}{
		{
			name: "debug log",
			logFunc: func() {
				logger.Debug("debug message", slog.String("field", "value"))
			},
			level:    "DEBUG",
			message:  "debug message",
			hasField: true,
			field:    "field",
		},
		{
			name: "info log",
			logFunc: func() {
				logger.Info("info message", slog.Int("count", 42))
			},
			level:    "INFO",
			message:  "info message",
			hasField: true,
			field:    "count",
		},
		{
			name: "warn log",
			logFunc: func() {
				logger.Warn("warning message")
			},
			level:    "WARN",
			message:  "warning message",
			hasField: false,
		},
		{
			name: "error log",
			logFunc: func() {
				logger.Error("error message", slog.String("error", "test error"))
			},
			level:    "ERROR",
			message:  "error message",
			hasField: true,
			field:    "error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf.Reset()
			tt.logFunc()

			output := buf.String()
			if output == "" {
				t.Errorf("Expected log output, got empty string")
				return
			}

			// Parse JSON log entry
			var logEntry map[string]interface{}
			if err := json.Unmarshal([]byte(output), &logEntry); err != nil {
				t.Errorf("Failed to parse JSON log output: %v", err)
				return
			}

			// Check level
			if level, ok := logEntry["level"].(string); !ok || level != tt.level {
				t.Errorf("Expected level %v, got %v", tt.level, level)
			}

			// Check message
			if msg, ok := logEntry["msg"].(string); !ok || msg != tt.message {
				t.Errorf("Expected message %q, got %q", tt.message, msg)
			}

			// Check field if expected
			if tt.hasField {
				if _, ok := logEntry[tt.field]; !ok {
					t.Errorf("Expected field %q not found in log entry", tt.field)
				}
			}
		})
	}
}

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
			isRedacted: false, // Empty strings are preserved as empty
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeSecrets(nil, tt.attr)

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

func TestStructuredLogger_With(t *testing.T) {
	var buf bytes.Buffer

	config := &LoggerConfig{
		Level:  LevelInfo,
		Format: "json",
	}

	opts := &slog.HandlerOptions{
		Level:       config.Level.ToSlogLevel(),
		ReplaceAttr: sanitizeSecrets,
	}
	handler := slog.NewJSONHandler(&buf, opts)
	logger := &StructuredLogger{
		logger: slog.New(handler),
		config: config,
	}

	// Create logger with additional context
	contextLogger := logger.With(
		slog.String("component", "test"),
		slog.Int("version", 1),
	)

	contextLogger.Info("test message")

	output := buf.String()
	if !strings.Contains(output, `"component":"test"`) {
		t.Errorf("Expected log to contain component field")
	}
	if !strings.Contains(output, `"version":1`) {
		t.Errorf("Expected log to contain version field")
	}
}

func TestStructuredLogger_WithContext(t *testing.T) {
	var buf bytes.Buffer

	config := &LoggerConfig{
		Level:  LevelInfo,
		Format: "json",
	}

	opts := &slog.HandlerOptions{
		Level:       config.Level.ToSlogLevel(),
		ReplaceAttr: sanitizeSecrets,
	}
	handler := slog.NewJSONHandler(&buf, opts)
	logger := &StructuredLogger{
		logger: slog.New(handler),
		config: config,
	}

	// Create context with deadline
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(time.Hour))
	defer cancel()

	contextLogger := logger.WithContext(ctx)
	contextLogger.Info("test message")

	output := buf.String()
	if !strings.Contains(output, `"deadline":`) {
		t.Errorf("Expected log to contain deadline field")
	}
}

// Benchmark tests for performance validation
func BenchmarkStructuredLogger_Info(b *testing.B) {
	logger, err := NewStructuredLogger(&LoggerConfig{
		Level:  LevelInfo,
		Format: "json",
	})
	if err != nil {
		b.Fatalf("Failed to create logger: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.Info("benchmark message", slog.Int("iteration", i))
	}
}

func BenchmarkSanitizeSecrets(b *testing.B) {
	attr := slog.String("password", "secret123456789")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sanitizeSecrets(nil, attr)
	}
}
