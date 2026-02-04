package infrastructure

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	adapterrepository "codeberg.org/brennenstuhl/uac-socializer/internal/adapters/repository"
	"codeberg.org/brennenstuhl/uac-socializer/internal/app"
	"codeberg.org/brennenstuhl/uac-socializer/internal/domain"
)

// createTestConfig creates a test configuration for use in container tests.
func createTestConfig(dataPath string) *app.Config {
	return &app.Config{
		DataPath:            dataPath,
		PromptsPath:         "./internal/infrastructure/prompts",
		MistralAPIKey:       "test-key",
		MistralModel:        "mistral-large",
		MistralMaxTokens:    1000,
		MistralTemperature:  0.7,
		BlueskyHandle:       "test.bsky.social",
		BlueskyPassword:     "test-password",
		MastodonServer:      "https://mastodon.social",
		MastodonAccessToken: "test-token",
		AIRetryCount:        3,
		AITimeout:           30 * time.Second,
		MinRetryCount:       0,
		MaxRetryCount:       10,
	}
}

// Config validation is now tested in app/config_test.go
// No need to duplicate those tests here

// Config sanitization is now tested in app/config_test.go
// No need to duplicate those tests here

func TestPlaceholderRepository(t *testing.T) {
	ctx := context.Background()
	repo := &placeholderRepository{}

	t.Run("GetPostForDate returns not implemented", func(t *testing.T) {
		post, err := repo.GetPostForDate(ctx, "01-01")
		assert.Nil(t, post)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not implemented")
		assert.Contains(t, err.Error(), "GetPostForDate")
	})

	t.Run("ListAvailableDates returns not implemented", func(t *testing.T) {
		dates, err := repo.ListAvailableDates(ctx)
		assert.Nil(t, dates)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not implemented")
		assert.Contains(t, err.Error(), "ListAvailableDates")
	})

	t.Run("GetPostCount returns not implemented", func(t *testing.T) {
		count, err := repo.GetPostCount(ctx)
		assert.Equal(t, 0, count)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not implemented")
		assert.Contains(t, err.Error(), "GetPostCount")
	})

	t.Run("ValidateDataSource returns not implemented", func(t *testing.T) {
		err := repo.ValidateDataSource(ctx)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not implemented")
		assert.Contains(t, err.Error(), "ValidateDataSource")
	})
}

func TestPlaceholderPublisher(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		platform              string
		expectedCharLimit     int
		expectedSupportsHash  bool
		expectedSupportsMedia bool
	}{
		{
			platform:              domain.PlatformBluesky,
			expectedCharLimit:     domain.BlueskyCharacterLimit,
			expectedSupportsHash:  true,
			expectedSupportsMedia: true,
		},
		{
			platform:              domain.PlatformMastodon,
			expectedCharLimit:     domain.MastodonCharacterLimit,
			expectedSupportsHash:  true,
			expectedSupportsMedia: true,
		},
		{
			platform:              "unknown",
			expectedCharLimit:     280,
			expectedSupportsHash:  true,
			expectedSupportsMedia: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.platform, func(t *testing.T) {
			publisher := newPlaceholderPublisher(tt.platform)

			assert.Equal(t, tt.platform, publisher.Platform())
			assert.Equal(t, tt.expectedCharLimit, publisher.GetCharacterLimit())
			assert.Equal(t, tt.expectedSupportsHash, publisher.SupportsHashtags())
			assert.Equal(t, tt.expectedSupportsMedia, publisher.SupportsMedia())

			// Test methods that should return not implemented
			result, err := publisher.Publish(ctx, nil)
			assert.Nil(t, result)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "not implemented")
			assert.Contains(t, err.Error(), tt.platform)

			err = publisher.ValidateContent(ctx, nil)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "not implemented")
			assert.Contains(t, err.Error(), tt.platform)

			err = publisher.TestConnection(ctx)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "not implemented")
			assert.Contains(t, err.Error(), tt.platform)
		})
	}
}

func TestPlaceholderGenerator(t *testing.T) {
	ctx := context.Background()
	generator := &placeholderGenerator{}

	t.Run("GenerateContent returns not implemented", func(t *testing.T) {
		content, err := generator.GenerateContent(ctx, nil)
		assert.Nil(t, content)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not implemented")
		assert.Contains(t, err.Error(), "GenerateContent")
	})

	t.Run("GenerateForAllPlatforms returns not implemented", func(t *testing.T) {
		contents, err := generator.GenerateForAllPlatforms(ctx, nil, "01-01")
		assert.Nil(t, contents)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not implemented")
		assert.Contains(t, err.Error(), "GenerateForAllPlatforms")
	})

	t.Run("ValidatePrompt returns not implemented", func(t *testing.T) {
		err := generator.ValidatePrompt(ctx, "bluesky", "test prompt")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not implemented")
		assert.Contains(t, err.Error(), "ValidatePrompt")
	})

	t.Run("GetSupportedModels returns not implemented", func(t *testing.T) {
		models, err := generator.GetSupportedModels(ctx)
		assert.Nil(t, models)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not implemented")
		assert.Contains(t, err.Error(), "GetSupportedModels")
	})

	t.Run("TestConnection returns not implemented", func(t *testing.T) {
		err := generator.TestConnection(ctx)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not implemented")
		assert.Contains(t, err.Error(), "TestConnection")
	})
}

func TestNewContainer_ValidConfig(t *testing.T) {
	// Create a temporary data file
	tmpDir := t.TempDir()
	dataPath := filepath.Join(tmpDir, "posts.json")
	err := os.WriteFile(dataPath, []byte(`{"01-01":[]}`), 0644)
	require.NoError(t, err)

	testConfig := createTestConfig(dataPath)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	container, err := NewContainer(ctx, testConfig)

	assert.NoError(t, err)
	assert.NotNil(t, container)
	assert.NotNil(t, container.Config)
	assert.NotNil(t, container.PostRepository)
	assert.NotNil(t, container.BlueskyPublisher)
	assert.NotNil(t, container.MastodonPublisher)
	assert.NotNil(t, container.ContentGenerator)

	// Verify config values
	assert.Equal(t, dataPath, container.Config.DataPath)

	// Verify publishers have correct platforms
	assert.Equal(t, domain.PlatformBluesky, container.BlueskyPublisher.Platform())
	assert.Equal(t, domain.PlatformMastodon, container.MastodonPublisher.Platform())
}

func TestNewContainer_InvalidConfig(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	t.Run("nil config", func(t *testing.T) {
		container, err := NewContainer(ctx, nil)

		assert.Error(t, err)
		assert.Nil(t, container)
		assert.Contains(t, err.Error(), "config cannot be nil")
	})

	t.Run("invalid config data", func(t *testing.T) {
		invalidConfig := createTestConfig("./data/posts_by_date.json")
		invalidConfig.MistralTemperature = 1.5 // Invalid: must be between 0.0 and 1.0

		container, err := NewContainer(ctx, invalidConfig)

		assert.Error(t, err)
		assert.Nil(t, container)
		assert.Contains(t, err.Error(), "invalid configuration")
	})
}

func TestContainer_GetRepository_ReturnsCorrectType(t *testing.T) {
	// Create a temporary data file
	tmpDir := t.TempDir()
	dataPath := filepath.Join(tmpDir, "posts.json")
	err := os.WriteFile(dataPath, []byte(`{"01-01":[]}`), 0644)
	require.NoError(t, err)

	testConfig := createTestConfig(dataPath)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	container, err := NewContainer(ctx, testConfig)
	require.NoError(t, err)

	repo := container.GetRepository()
	assert.NotNil(t, repo)

	// Verify it's the real JSONFileRepository implementation (not placeholder)
	_, ok := repo.(*adapterrepository.JSONFileRepository)
	assert.True(t, ok, "repository should be JSONFileRepository, not placeholder")
}

func TestContainer_GetPublisher(t *testing.T) {
	// Create a temporary data file
	tmpDir := t.TempDir()
	dataPath := filepath.Join(tmpDir, "posts.json")
	err := os.WriteFile(dataPath, []byte(`{"01-01":[]}`), 0644)
	require.NoError(t, err)

	testConfig := createTestConfig(dataPath)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	container, err := NewContainer(ctx, testConfig)
	require.NoError(t, err)

	tests := []struct {
		platform    string
		expectError bool
		errorMsg    string
	}{
		{
			platform:    domain.PlatformBluesky,
			expectError: false,
		},
		{
			platform:    domain.PlatformMastodon,
			expectError: false,
		},
		{
			platform:    "BLUESKY", // Test case insensitivity
			expectError: false,
		},
		{
			platform:    " mastodon ", // Test trimming
			expectError: false,
		},
		{
			platform:    "invalid",
			expectError: true,
			errorMsg:    "unsupported platform",
		},
	}

	for _, tt := range tests {
		t.Run(tt.platform, func(t *testing.T) {
			publisher, err := container.GetPublisher(tt.platform)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, publisher)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, publisher)
				// Normalize platform for comparison
				expectedPlatform := strings.ToLower(strings.TrimSpace(tt.platform))
				assert.Equal(t, expectedPlatform, publisher.Platform())
			}
		})
	}
}

func TestContainer_Validate_Success(t *testing.T) {
	// Create a temporary data file
	tmpDir := t.TempDir()
	dataPath := filepath.Join(tmpDir, "posts.json")
	err := os.WriteFile(dataPath, []byte(`{"01-01":[]}`), 0644)
	require.NoError(t, err)

	testConfig := createTestConfig(dataPath)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	container, err := NewContainer(ctx, testConfig)
	require.NoError(t, err)

	// Validation should succeed (warnings are expected for placeholders)
	err = container.Validate(ctx)
	assert.NoError(t, err)
}

func TestContainer_Validate_MissingDataSource(t *testing.T) {
	testConfig := createTestConfig("/nonexistent/path/posts.json")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// NewContainer should now fail with missing data file (real repository validates during init)
	// This ensures we catch data access issues early rather than during operations
	container, err := NewContainer(ctx, testConfig)
	assert.Error(t, err)
	assert.Nil(t, container)
	assert.Contains(t, err.Error(), "failed to initialize repository")
}

func TestContainer_Close_NoError(t *testing.T) {
	// Create a temporary data file
	tmpDir := t.TempDir()
	dataPath := filepath.Join(tmpDir, "posts.json")
	err := os.WriteFile(dataPath, []byte(`{"01-01":[]}`), 0644)
	require.NoError(t, err)

	testConfig := createTestConfig(dataPath)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	container, err := NewContainer(ctx, testConfig)
	require.NoError(t, err)

	err = container.Close()
	assert.NoError(t, err)
}

func TestContainer_GetMethods(t *testing.T) {
	// Create a temporary data file
	tmpDir := t.TempDir()
	dataPath := filepath.Join(tmpDir, "posts.json")
	err := os.WriteFile(dataPath, []byte(`{"01-01":[]}`), 0644)
	require.NoError(t, err)

	testConfig := createTestConfig(dataPath)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	container, err := NewContainer(ctx, testConfig)
	require.NoError(t, err)

	t.Run("GetContentGenerator", func(t *testing.T) {
		generator := container.GetContentGenerator()
		assert.NotNil(t, generator)
		_, ok := generator.(*placeholderGenerator)
		assert.True(t, ok)
	})

	t.Run("GetConfig", func(t *testing.T) {
		config := container.GetConfig()
		assert.NotNil(t, config)
		assert.Equal(t, container.Config, config)
	})
}
