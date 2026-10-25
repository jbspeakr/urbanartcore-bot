package app

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config holds all application configuration with environment variable binding.
// All secrets and runtime configuration are provided via UAC_* environment variables
// following 12-factor app principles.
type Config struct {
	// Core configuration
	DataPath    string `mapstructure:"data_path"`
	PromptsPath string `mapstructure:"prompts_path"`

	// Mistral AI configuration
	MistralAPIKey      string  `mapstructure:"mistral_api_key"`
	MistralModel       string  `mapstructure:"mistral_model"`
	MistralMaxTokens   int     `mapstructure:"mistral_max_tokens"`
	MistralTemperature float64 `mapstructure:"mistral_temperature"`

	// Bluesky configuration
	BlueskyHandle   string `mapstructure:"bluesky_handle"`
	BlueskyPassword string `mapstructure:"bluesky_password"`

	// Mastodon configuration
	MastodonServer      string `mapstructure:"mastodon_server"`
	MastodonAccessToken string `mapstructure:"mastodon_access_token"`

	// Operational configuration
	AIRetryCount  int           `mapstructure:"ai_retry_count"`
	AITimeout     time.Duration `mapstructure:"ai_timeout"`
	MaxRetryCount int           `mapstructure:"max_retry_count"`
	MinRetryCount int           `mapstructure:"min_retry_count"`
}

// ConfigError represents configuration-related errors.
type ConfigError struct {
	Err error // Underlying error
}

func (e ConfigError) Error() string {
	return e.Err.Error()
}

func (e ConfigError) Unwrap() error {
	return e.Err
}

// Common configuration errors
var (
	ErrInvalidTemperature   = errors.New("invalid temperature")
	ErrInvalidTimeout       = errors.New("invalid timeout")
	ErrMissingRequiredField = errors.New("missing required field")
)

// LoadConfig loads configuration from environment variables using Viper.
// It sets defaults for optional fields and validates all configuration values.
// Returns error only for configuration loading failures, not missing optional values.
func LoadConfig() (*Config, error) {
	v := viper.New()

	// Configure Viper for environment variables
	v.SetEnvPrefix("UAC")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Set defaults BEFORE binding to avoid conflicts
	v.SetDefault("data_path", "./data/posts_by_date.json")
	v.SetDefault("prompts_path", "./internal/infrastructure/prompts")
	v.SetDefault("mistral_model", "mistral-large-2512")
	v.SetDefault("mistral_max_tokens", 1000)
	v.SetDefault("mistral_temperature", 0.7)
	v.SetDefault("ai_retry_count", 3)
	v.SetDefault("ai_timeout", "30s")
	v.SetDefault("max_retry_count", 10)
	v.SetDefault("min_retry_count", 0)

	// Bind all environment variables after setting defaults
	envVars := []string{
		"data_path",
		"prompts_path",
		"mistral_api_key",
		"mistral_model",
		"mistral_max_tokens",
		"mistral_temperature",
		"bluesky_handle",
		"bluesky_password",
		"mastodon_server",
		"mastodon_access_token",
		"ai_retry_count",
		"ai_timeout",
		"max_retry_count",
		"min_retry_count",
	}

	for _, envVar := range envVars {
		if err := v.BindEnv(envVar); err != nil {
			return nil, ConfigError{Err: fmt.Errorf("failed to bind env var %s: %w", envVar, err)}
		}
	}

	// Parse duration string first before unmarshal to provide better error messages
	if timeoutStr := v.GetString("ai_timeout"); timeoutStr != "" {
		duration, err := time.ParseDuration(timeoutStr)
		if err != nil {
			return nil, ConfigError{Err: fmt.Errorf("invalid ai_timeout format: %w", err)}
		}
		v.Set("ai_timeout", duration)
	}

	var config Config
	if err := v.Unmarshal(&config); err != nil {
		return nil, ConfigError{Err: fmt.Errorf("failed to unmarshal config: %w", err)}
	}

	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, ConfigError{Err: err}
	}

	return &config, nil
}

// Validate performs validation on all configuration fields.
// It checks for valid temperature ranges, and positive timeouts.
// Returns detailed error messages indicating which field failed validation.
func (c *Config) Validate() error {
	// Validate retry count bounds are properly configured
	if c.MinRetryCount < 0 {
		return ConfigError{
			Err: fmt.Errorf("min_retry_count must be non-negative (got: %d)", c.MinRetryCount),
		}
	}

	if c.MaxRetryCount < c.MinRetryCount {
		return ConfigError{
			Err: fmt.Errorf("max_retry_count (%d) must be >= min_retry_count (%d)", c.MaxRetryCount, c.MinRetryCount),
		}
	}

	// Validate Mistral temperature (0.0 to 1.0)
	if c.MistralTemperature < 0.0 || c.MistralTemperature > 1.0 {
		return ConfigError{
			Err: fmt.Errorf("%w: mistral_temperature must be between 0.0 and 1.0 (got: %f)", ErrInvalidTemperature, c.MistralTemperature),
		}
	}

	// Validate AI timeout is positive
	if c.AITimeout <= 0 {
		return ConfigError{
			Err: fmt.Errorf("%w: ai_timeout must be positive (got: %v)", ErrInvalidTimeout, c.AITimeout),
		}
	}

	// Validate AI retry count bounds
	if c.AIRetryCount < c.MinRetryCount || c.AIRetryCount > c.MaxRetryCount {
		return ConfigError{
			Err: fmt.Errorf("ai_retry_count must be between %d and %d (got: %d)", c.MinRetryCount, c.MaxRetryCount, c.AIRetryCount),
		}
	}

	// Validate Mistral max tokens is positive
	if c.MistralMaxTokens <= 0 {
		return ConfigError{
			Err: fmt.Errorf("mistral_max_tokens must be positive (got: %d)", c.MistralMaxTokens),
		}
	}

	return nil
}

// SanitizeForLogging returns a copy of the configuration with all secrets redacted.
// This is used for safe logging without exposing sensitive credentials.
func (c *Config) SanitizeForLogging() Config {
	sanitized := *c

	// Redact all secrets
	if sanitized.MistralAPIKey != "" {
		sanitized.MistralAPIKey = "***"
	}
	if sanitized.BlueskyPassword != "" {
		sanitized.BlueskyPassword = "***"
	}
	if sanitized.MastodonAccessToken != "" {
		sanitized.MastodonAccessToken = "***"
	}

	return sanitized
}

// GetMistralConfig returns Mistral-specific configuration as a separate struct.
// This is useful for passing only relevant configuration to the Mistral adapter.
func (c *Config) GetMistralConfig() MistralConfig {
	return MistralConfig{
		APIKey:      c.MistralAPIKey,
		Model:       c.MistralModel,
		MaxTokens:   c.MistralMaxTokens,
		Temperature: c.MistralTemperature,
		RetryCount:  c.AIRetryCount,
		Timeout:     c.AITimeout,
	}
}

// GetBlueskyConfig returns Bluesky-specific configuration as a separate struct.
func (c *Config) GetBlueskyConfig() BlueskyConfig {
	return BlueskyConfig{
		Handle:   c.BlueskyHandle,
		Password: c.BlueskyPassword,
	}
}

// GetMastodonConfig returns Mastodon-specific configuration as a separate struct.
func (c *Config) GetMastodonConfig() MastodonConfig {
	return MastodonConfig{
		Server:      c.MastodonServer,
		AccessToken: c.MastodonAccessToken,
	}
}

// MistralConfig contains Mistral AI-specific configuration.
type MistralConfig struct {
	APIKey      string
	Model       string
	MaxTokens   int
	Temperature float64
	RetryCount  int
	Timeout     time.Duration
	Endpoint    string // Optional API base URL; defaults to the public Mistral API
}

// BlueskyConfig contains Bluesky-specific configuration.
type BlueskyConfig struct {
	Handle   string
	Password string
}

// MastodonConfig contains Mastodon-specific configuration.
type MastodonConfig struct {
	Server      string
	AccessToken string
}
