package app

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig_AllEnvVars(t *testing.T) {
	// Set all environment variables
	envVars := map[string]string{
		"UAC_SITE_BASE_URL":         "https://archive.example.org",
		"UAC_PROMPTS_PATH":          "/custom/prompts",
		"UAC_MISTRAL_API_KEY":       "sk-test-key-123",
		"UAC_MISTRAL_MODEL":         "mistral-medium",
		"UAC_MISTRAL_MAX_TOKENS":    "2000",
		"UAC_MISTRAL_TEMPERATURE":   "0.5",
		"UAC_BLUESKY_HANDLE":        "test.bsky.social",
		"UAC_BLUESKY_PASSWORD":      "test-password",
		"UAC_MASTODON_SERVER":       "https://test.mastodon",
		"UAC_MASTODON_ACCESS_TOKEN": "test-token-123",
		"UAC_AI_RETRY_COUNT":        "5",
		"UAC_AI_TIMEOUT":            "45s",
		"UAC_MAX_RETRY_COUNT":       "15",
		"UAC_MIN_RETRY_COUNT":       "1",
	}

	// Set environment variables
	for key, value := range envVars {
		t.Setenv(key, value)
	}

	config, err := LoadConfig()
	require.NoError(t, err)
	require.NotNil(t, config)

	// Verify all values are loaded correctly
	assert.Equal(t, "https://archive.example.org", config.SiteBaseURL)
	assert.Equal(t, "/custom/prompts", config.PromptsPath)
	assert.Equal(t, "sk-test-key-123", config.MistralAPIKey)
	assert.Equal(t, "mistral-medium", config.MistralModel)
	assert.Equal(t, 2000, config.MistralMaxTokens)
	assert.Equal(t, 0.5, config.MistralTemperature)
	assert.Equal(t, "test.bsky.social", config.BlueskyHandle)
	assert.Equal(t, "test-password", config.BlueskyPassword)
	assert.Equal(t, "https://test.mastodon", config.MastodonServer)
	assert.Equal(t, "test-token-123", config.MastodonAccessToken)
	assert.Equal(t, 5, config.AIRetryCount)
	assert.Equal(t, 45*time.Second, config.AITimeout)
	assert.Equal(t, 15, config.MaxRetryCount)
	assert.Equal(t, 1, config.MinRetryCount)
}

func TestLoadConfig_Defaults(t *testing.T) {
	// Clear all UAC environment variables to test defaults
	clearUACEnvVars()

	config, err := LoadConfig()
	require.NoError(t, err)
	require.NotNil(t, config)

	// Verify default values
	assert.Equal(t, "https://www.urbanartcore.com", config.SiteBaseURL)
	assert.Equal(t, "./internal/infrastructure/prompts", config.PromptsPath)
	assert.Equal(t, "mistral-large-2512", config.MistralModel)
	assert.Equal(t, "ministral-8b-2512", config.CriticModel)
	assert.Equal(t, 1000, config.MistralMaxTokens)
	assert.Equal(t, 0.7, config.MistralTemperature)
	assert.Equal(t, 3, config.AIRetryCount)
	assert.Equal(t, 30*time.Second, config.AITimeout)

	// Optional fields should be empty when not set
	assert.Equal(t, "", config.MistralAPIKey)
	assert.Equal(t, "", config.BlueskyHandle)
	assert.Equal(t, "", config.BlueskyPassword)
	assert.Equal(t, "", config.MastodonServer)
	assert.Equal(t, "", config.MastodonAccessToken)
}

func TestLoadConfig_InvalidDuration(t *testing.T) {
	clearUACEnvVars()
	t.Setenv("UAC_AI_TIMEOUT", "invalid-duration")

	_, err := LoadConfig()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid ai_timeout format")
}

func TestValidateConfig_Success(t *testing.T) {
	config := &Config{
		MistralTemperature: 0.7,
		AITimeout:          30 * time.Second,
		AIRetryCount:       3,
		MistralMaxTokens:   1000,
		MinRetryCount:      0,
		MaxRetryCount:      10,
	}

	err := config.Validate()
	assert.NoError(t, err)
}

func TestValidateConfig_UnpinnedModel(t *testing.T) {
	config := &Config{
		MistralModel:       "mistral-large-latest",
		MistralTemperature: 0.7,
		AITimeout:          30 * time.Second,
		AIRetryCount:       3,
		MistralMaxTokens:   1000,
		MaxRetryCount:      10,
	}

	err := config.Validate()
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUnpinnedModel))

	config.MistralModel = "mistral-large-2512"
	assert.NoError(t, config.Validate())

	config.CriticModel = "ministral-8b-latest"
	assert.True(t, errors.Is(config.Validate(), ErrUnpinnedModel), "the critic's model must be pinned too")
	config.CriticModel = "ministral-8b-2512"
	assert.NoError(t, config.Validate())
}

func TestValidateConfig_InvalidTemperature(t *testing.T) {
	testCases := []struct {
		name        string
		temperature float64
	}{
		{"negative", -0.1},
		{"too_high", 1.1},
		{"way_too_high", 2.0},
		{"zero_valid", 0.0},
		{"one_valid", 1.0},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			config := &Config{
				MistralTemperature: tc.temperature,
				AITimeout:          30 * time.Second,
				AIRetryCount:       3,
				MistralMaxTokens:   1000,
				MinRetryCount:      0,
				MaxRetryCount:      10,
			}

			err := config.Validate()

			if tc.temperature >= 0.0 && tc.temperature <= 1.0 {
				assert.NoError(t, err, "Temperature %f should be valid", tc.temperature)
			} else {
				require.Error(t, err, "Temperature %f should be invalid", tc.temperature)
				assert.Contains(t, err.Error(), "invalid temperature")
			}
		})
	}
}

func TestValidateConfig_InvalidTimeout(t *testing.T) {
	testCases := []struct {
		name    string
		timeout time.Duration
	}{
		{"zero", 0},
		{"negative", -1 * time.Second},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			config := &Config{
				MistralTemperature: 0.7,
				AITimeout:          tc.timeout,
				AIRetryCount:       3,
				MistralMaxTokens:   1000,
				MinRetryCount:      0,
				MaxRetryCount:      10,
			}

			err := config.Validate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), "invalid timeout")
		})
	}
}

func TestValidateConfig_InvalidRetryCount(t *testing.T) {
	config := &Config{
		MistralTemperature: 0.7,
		AITimeout:          30 * time.Second,
		AIRetryCount:       -1,
		MistralMaxTokens:   1000,
		MinRetryCount:      0,
		MaxRetryCount:      10,
	}

	err := config.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ai_retry_count must be between 0 and 10")
}

func TestValidateConfig_InvalidMaxTokens(t *testing.T) {
	testCases := []struct {
		name      string
		maxTokens int
	}{
		{"zero", 0},
		{"negative", -100},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			config := &Config{
				MistralTemperature: 0.7,
				AITimeout:          30 * time.Second,
				AIRetryCount:       3,
				MistralMaxTokens:   tc.maxTokens,
				MinRetryCount:      0,
				MaxRetryCount:      10,
			}

			err := config.Validate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), "mistral_max_tokens must be positive")
		})
	}
}

func TestValidateConfig_InvalidRetryBounds(t *testing.T) {
	testCases := []struct {
		name          string
		minRetryCount int
		maxRetryCount int
		expectedError string
	}{
		{"negative_min", -1, 10, "min_retry_count must be non-negative"},
		{"max_less_than_min", 5, 3, "must be >="}, // Match substring since full message includes values
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			config := &Config{
				MistralTemperature: 0.7,
				AITimeout:          30 * time.Second,
				AIRetryCount:       3,
				MistralMaxTokens:   1000,
				MinRetryCount:      tc.minRetryCount,
				MaxRetryCount:      tc.maxRetryCount,
			}

			err := config.Validate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.expectedError)
		})
	}
}

func TestValidateConfig_ValidRetryBounds(t *testing.T) {
	testCases := []struct {
		name          string
		minRetryCount int
		maxRetryCount int
		aiRetryCount  int
	}{
		{"default_bounds", 0, 10, 3},
		{"custom_bounds", 1, 5, 3},
		{"zero_min", 0, 10, 0},
		{"at_max", 0, 10, 10},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			config := &Config{
				MistralTemperature: 0.7,
				AITimeout:          30 * time.Second,
				AIRetryCount:       tc.aiRetryCount,
				MistralMaxTokens:   1000,
				MinRetryCount:      tc.minRetryCount,
				MaxRetryCount:      tc.maxRetryCount,
			}

			err := config.Validate()
			require.NoError(t, err)
		})
	}
}

func TestSanitizeForLogging_RedactsSecrets(t *testing.T) {
	config := &Config{
		SiteBaseURL:         "https://www.urbanartcore.com",
		MistralAPIKey:       "sk-secret-key-12345",
		BlueskyPassword:     "secret-password",
		MastodonAccessToken: "access-token-67890",
		MistralModel:        "mistral-large",
	}

	sanitized := config.SanitizeForLogging()

	// Non-secrets should be preserved
	assert.Equal(t, config.SiteBaseURL, sanitized.SiteBaseURL)
	assert.Equal(t, config.MistralModel, sanitized.MistralModel)

	// Secrets should be redacted
	assert.Equal(t, "***", sanitized.MistralAPIKey)
	assert.Equal(t, "***", sanitized.BlueskyPassword)
	assert.Equal(t, "***", sanitized.MastodonAccessToken)

	// Original config should be unchanged
	assert.Equal(t, "sk-secret-key-12345", config.MistralAPIKey)
	assert.Equal(t, "secret-password", config.BlueskyPassword)
	assert.Equal(t, "access-token-67890", config.MastodonAccessToken)
}

func TestSanitizeForLogging_EmptySecrets(t *testing.T) {
	config := &Config{
		SiteBaseURL:         "https://www.urbanartcore.com",
		MistralAPIKey:       "",
		BlueskyPassword:     "",
		MastodonAccessToken: "",
	}

	sanitized := config.SanitizeForLogging()

	// Empty secrets should remain empty
	assert.Equal(t, "", sanitized.MistralAPIKey)
	assert.Equal(t, "", sanitized.BlueskyPassword)
	assert.Equal(t, "", sanitized.MastodonAccessToken)
	assert.Equal(t, config.SiteBaseURL, sanitized.SiteBaseURL)
}

func TestGetMistralConfig(t *testing.T) {
	config := &Config{
		MistralAPIKey:      "test-key",
		MistralModel:       "mistral-medium",
		MistralMaxTokens:   2000,
		MistralTemperature: 0.5,
		AIRetryCount:       5,
		AITimeout:          45 * time.Second,
	}

	mistralConfig := config.GetMistralConfig()

	assert.Equal(t, "test-key", mistralConfig.APIKey)
	assert.Equal(t, "mistral-medium", mistralConfig.Model)
	assert.Equal(t, 2000, mistralConfig.MaxTokens)
	assert.Equal(t, 0.5, mistralConfig.Temperature)
	assert.Equal(t, 5, mistralConfig.RetryCount)
	assert.Equal(t, 45*time.Second, mistralConfig.Timeout)
}

func TestGetBlueskyConfig(t *testing.T) {
	config := &Config{
		BlueskyHandle:   "test.bsky.social",
		BlueskyPassword: "test-password",
	}

	blueskyConfig := config.GetBlueskyConfig()

	assert.Equal(t, "test.bsky.social", blueskyConfig.Handle)
	assert.Equal(t, "test-password", blueskyConfig.Password)
}

func TestGetMastodonConfig(t *testing.T) {
	config := &Config{
		MastodonServer:      "https://mastodon.social",
		MastodonAccessToken: "test-token",
	}

	mastodonConfig := config.GetMastodonConfig()

	assert.Equal(t, "https://mastodon.social", mastodonConfig.Server)
	assert.Equal(t, "test-token", mastodonConfig.AccessToken)
}

func TestConfigError(t *testing.T) {
	baseErr := assert.AnError
	configErr := ConfigError{Err: baseErr}

	assert.Equal(t, baseErr.Error(), configErr.Error())
	assert.Equal(t, baseErr, configErr.Unwrap())
}

// Helper function to clear all UAC environment variables for testing defaults
func clearUACEnvVars() {
	envVars := []string{
		"UAC_SITE_BASE_URL",
		"UAC_PROMPTS_PATH",
		"UAC_MISTRAL_API_KEY",
		"UAC_MISTRAL_MODEL",
		"UAC_MISTRAL_MAX_TOKENS",
		"UAC_MISTRAL_TEMPERATURE",
		"UAC_BLUESKY_HANDLE",
		"UAC_BLUESKY_PASSWORD",
		"UAC_MASTODON_SERVER",
		"UAC_MASTODON_ACCESS_TOKEN",
		"UAC_AI_RETRY_COUNT",
		"UAC_AI_TIMEOUT",
	}

	for _, envVar := range envVars {
		// Use os.Unsetenv since we want to actually clear them
		_ = os.Unsetenv(envVar)
	}
}
