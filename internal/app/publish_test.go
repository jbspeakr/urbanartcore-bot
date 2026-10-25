package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/jbspeakr/urbanartcore-bot/internal/domain"
	"github.com/jbspeakr/urbanartcore-bot/internal/ports"
)

// Mock implementations for testing

// MockPostRepository implements ports.PostRepository for testing
type MockPostRepository struct {
	mock.Mock
}

func (m *MockPostRepository) GetPostForDate(ctx context.Context, dateKey string) (*domain.Post, error) {
	args := m.Called(ctx, dateKey)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Post), args.Error(1)
}

func (m *MockPostRepository) ListAvailableDates(ctx context.Context) ([]string, error) {
	args := m.Called(ctx)
	return args.Get(0).([]string), args.Error(1)
}

func (m *MockPostRepository) GetPostCount(ctx context.Context) (int, error) {
	args := m.Called(ctx)
	return args.Get(0).(int), args.Error(1)
}

func (m *MockPostRepository) ValidateDataSource(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

// MockContentGenerator implements ports.ContentGenerator for testing
type MockContentGenerator struct {
	mock.Mock
}

func (m *MockContentGenerator) GenerateContent(ctx context.Context, req *ports.GenerationRequest) (*domain.GeneratedContent, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.GeneratedContent), args.Error(1)
}

func (m *MockContentGenerator) GenerateForAllPlatforms(ctx context.Context, posts []*domain.Post, dateKey string) (map[string]*domain.GeneratedContent, error) {
	args := m.Called(ctx, posts, dateKey)
	return args.Get(0).(map[string]*domain.GeneratedContent), args.Error(1)
}

func (m *MockContentGenerator) ValidatePrompt(ctx context.Context, platform, prompt string) error {
	args := m.Called(ctx, platform, prompt)
	return args.Error(0)
}

func (m *MockContentGenerator) TestConnection(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

// MockSocialPublisher implements ports.SocialPublisher for testing
type MockSocialPublisher struct {
	mock.Mock
	platform string
}

func NewMockSocialPublisher(platform string) *MockSocialPublisher {
	return &MockSocialPublisher{platform: platform}
}

func (m *MockSocialPublisher) Platform() string {
	return m.platform
}

func (m *MockSocialPublisher) Publish(ctx context.Context, content *domain.GeneratedContent) (*domain.PlatformResult, error) {
	args := m.Called(ctx, content)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.PlatformResult), args.Error(1)
}

func (m *MockSocialPublisher) ValidateContent(ctx context.Context, content *domain.GeneratedContent) error {
	args := m.Called(ctx, content)
	return args.Error(0)
}

func (m *MockSocialPublisher) GetCharacterLimit() int {
	args := m.Called()
	return args.Get(0).(int)
}

func (m *MockSocialPublisher) SupportsHashtags() bool {
	args := m.Called()
	return args.Get(0).(bool)
}

func (m *MockSocialPublisher) SupportsMedia() bool {
	args := m.Called()
	return args.Get(0).(bool)
}

func (m *MockSocialPublisher) TestConnection(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

// Test helper functions

func createTestPost(uri, dateStr string) *domain.Post {
	post, err := domain.NewPost(uri, dateStr)
	if err != nil {
		panic(err)
	}
	return post
}

func createTestGeneratedContent(text, platform string) *domain.GeneratedContent {
	content, err := domain.NewGeneratedContent(text, platform, []string{"#test"}, "test-prompt",
		"https://www.example.com/test")
	if err != nil {
		panic(err)
	}
	return content
}

func createTestPublishService(repo ports.PostRepository, gen ports.ContentGenerator, publishers map[string]ports.SocialPublisher) *PublishService {
	return NewPublishService(repo, gen, publishers)
}

// Test cases

func TestNewPublishService(t *testing.T) {
	repo := &MockPostRepository{}
	gen := &MockContentGenerator{}
	publishers := map[string]ports.SocialPublisher{
		"bluesky": NewMockSocialPublisher("bluesky"),
	}

	service := NewPublishService(repo, gen, publishers)

	assert.NotNil(t, service)
	assert.Equal(t, repo, service.repository)
	assert.Equal(t, gen, service.generator)
	assert.Equal(t, publishers, service.publishers)
}

func TestPublishAnniversaryPosts_HappyPath(t *testing.T) {
	ctx := context.Background()

	// Setup test data
	posts := []*domain.Post{
		createTestPost("https://example.com/post1", "2020-09-14"),
	}

	// Setup mocks
	repo := &MockPostRepository{}
	repo.On("GetPostForDate", ctx, "09-14").Return(posts[0], nil)

	gen := &MockContentGenerator{}
	blueskycontent := createTestGeneratedContent("Test bluesky content", "bluesky")
	gen.On("GenerateContent", ctx, mock.AnythingOfType("*ports.GenerationRequest")).Return(blueskycontent, nil)

	blueskyPub := NewMockSocialPublisher("bluesky")
	blueskyPub.On("ValidateContent", ctx, blueskycontent).Return(nil)

	publishResult, _ := domain.NewPlatformResult("bluesky", domain.StatusSuccess, blueskycontent)
	publishResult.SetSuccess("https://bsky.app/post/123", time.Millisecond*100)
	blueskyPub.On("Publish", ctx, blueskycontent).Return(publishResult, nil)

	publishers := map[string]ports.SocialPublisher{
		"bluesky": blueskyPub,
	}

	service := createTestPublishService(repo, gen, publishers)

	// Execute
	req := &PublishRequest{
		DateKey:   "09-14",
		Platforms: []string{"bluesky"},
		DryRun:    false,
	}

	response, err := service.PublishAnniversaryPosts(ctx, req)

	// Assertions
	require.NoError(t, err)
	require.NotNil(t, response)
	assert.NotNil(t, response.Result)
	assert.NotEmpty(t, response.Generated)
	assert.Nil(t, response.Error)

	assert.True(t, response.Result.IsSuccess())
	assert.Equal(t, 1, response.Result.SuccessCount())
	assert.Equal(t, 0, response.Result.FailureCount())

	// Verify mocks
	repo.AssertExpectations(t)
	gen.AssertExpectations(t)
	blueskyPub.AssertExpectations(t)
}

func TestPublishAnniversaryPosts_DryRun(t *testing.T) {
	ctx := context.Background()

	// Setup test data
	posts := []*domain.Post{
		createTestPost("https://example.com/post1", "2020-09-14"),
	}

	// Setup mocks
	repo := &MockPostRepository{}
	repo.On("GetPostForDate", ctx, "09-14").Return(posts[0], nil)

	gen := &MockContentGenerator{}
	// Placeholder generator returns "not implemented" error in dry-run
	gen.On("GenerateContent", ctx, mock.AnythingOfType("*ports.GenerationRequest")).
		Return(nil, errors.New("not implemented"))

	blueskyPub := NewMockSocialPublisher("bluesky")
	// Content validation should be called but may return "not implemented"
	blueskyPub.On("ValidateContent", ctx, mock.AnythingOfType("*domain.GeneratedContent")).
		Return(errors.New("not implemented"))

	publishers := map[string]ports.SocialPublisher{
		"bluesky": blueskyPub,
	}

	service := createTestPublishService(repo, gen, publishers)

	// Execute
	req := &PublishRequest{
		DateKey:   "09-14",
		Platforms: []string{"bluesky"},
		DryRun:    true,
	}

	response, err := service.PublishAnniversaryPosts(ctx, req)

	// Assertions
	require.NoError(t, err)
	require.NotNil(t, response)
	assert.NotNil(t, response.Result)
	assert.NotEmpty(t, response.Generated)

	// In dry-run, we should have mock content and skipped status
	assert.Equal(t, 1, len(response.Generated))
	mockContent := response.Generated["bluesky"]
	assert.NotNil(t, mockContent)
	assert.Contains(t, mockContent.Text(), "Anniversary post")

	platformResult := response.Result.GetPlatformResult("bluesky")
	assert.NotNil(t, platformResult)
	assert.Equal(t, domain.StatusSkipped, platformResult.Status())

	// Verify mocks
	repo.AssertExpectations(t)
	gen.AssertExpectations(t)
	blueskyPub.AssertExpectations(t)
}

func TestPublishAnniversaryPosts_PartialFailure(t *testing.T) {
	ctx := context.Background()

	// Setup test data
	posts := []*domain.Post{
		createTestPost("https://example.com/post1", "2020-09-14"),
	}

	// Setup mocks
	repo := &MockPostRepository{}
	repo.On("GetPostForDate", ctx, "09-14").Return(posts[0], nil)

	gen := &MockContentGenerator{}
	blueskyContent := createTestGeneratedContent("Test bluesky content", "bluesky")

	// First call for bluesky - success
	gen.On("GenerateContent", ctx, mock.MatchedBy(func(req *ports.GenerationRequest) bool {
		return req.Platform == "bluesky"
	})).Return(blueskyContent, nil)

	// Second call for mastodon - failure
	gen.On("GenerateContent", ctx, mock.MatchedBy(func(req *ports.GenerationRequest) bool {
		return req.Platform == "mastodon"
	})).Return(nil, errors.New("generation failed"))

	// Bluesky publisher - success
	blueskyPub := NewMockSocialPublisher("bluesky")
	blueskyPub.On("ValidateContent", ctx, blueskyContent).Return(nil)
	publishResult, _ := domain.NewPlatformResult("bluesky", domain.StatusSuccess, blueskyContent)
	publishResult.SetSuccess("https://bsky.app/post/123", time.Millisecond*100)
	blueskyPub.On("Publish", ctx, blueskyContent).Return(publishResult, nil)

	// Mastodon publisher - won't be called since generation failed
	mastodonPub := NewMockSocialPublisher("mastodon")

	publishers := map[string]ports.SocialPublisher{
		"bluesky":  blueskyPub,
		"mastodon": mastodonPub,
	}

	service := createTestPublishService(repo, gen, publishers)

	// Execute
	req := &PublishRequest{
		DateKey:   "09-14",
		Platforms: []string{"bluesky", "mastodon"},
		DryRun:    false,
	}

	response, err := service.PublishAnniversaryPosts(ctx, req)

	// Assertions
	require.NoError(t, err)
	require.NotNil(t, response)
	assert.NotNil(t, response.Result)

	// Should have partial failure
	assert.False(t, response.Result.IsSuccess())
	assert.True(t, response.Result.HasFailures())
	assert.Equal(t, 1, response.Result.SuccessCount())
	assert.Equal(t, 1, response.Result.FailureCount())
	assert.Equal(t, ErrPartialFailure, response.Error)

	// Should have one generated content (bluesky)
	assert.Equal(t, 1, len(response.Generated))
	assert.Contains(t, response.Generated, "bluesky")
	assert.NotContains(t, response.Generated, "mastodon")

	// Verify mocks
	repo.AssertExpectations(t)
	gen.AssertExpectations(t)
	blueskyPub.AssertExpectations(t)
	mastodonPub.AssertExpectations(t)
}

func TestPublishAnniversaryPosts_NoPostsFound(t *testing.T) {
	ctx := context.Background()

	// Setup mocks
	repo := &MockPostRepository{}
	repo.On("GetPostForDate", ctx, "09-14").Return(nil, ports.ErrDateNotFound)

	gen := &MockContentGenerator{}
	publishers := map[string]ports.SocialPublisher{
		"bluesky": NewMockSocialPublisher("bluesky"),
	}

	service := createTestPublishService(repo, gen, publishers)

	// Execute
	req := &PublishRequest{
		DateKey:   "09-14",
		Platforms: []string{"bluesky"},
		DryRun:    false,
	}

	response, err := service.PublishAnniversaryPosts(ctx, req)

	// Assertions: a date without posts is a rest day, not an error
	require.NoError(t, err)
	require.NotNil(t, response)
	assert.True(t, response.RestDay)
	assert.Nil(t, response.Result)
	assert.NoError(t, response.Error)

	// Verify mocks: nothing was generated or published
	repo.AssertExpectations(t)
	gen.AssertNotCalled(t, "GenerateContent", mock.Anything, mock.Anything)
}

func TestPublishAnniversaryPosts_InvalidDateValidation(t *testing.T) {
	service := createTestPublishService(nil, nil, nil)

	tests := []struct {
		name    string
		dateKey string
		wantErr string
	}{
		{
			name:    "empty date key",
			dateKey: "",
			wantErr: "date key cannot be empty",
		},
		{
			name:    "invalid format - wrong separator",
			dateKey: "09/14",
			wantErr: "expected MM-DD format",
		},
		{
			name:    "invalid format - single digit month",
			dateKey: "9-14",
			wantErr: "expected MM-DD format",
		},
		{
			name:    "invalid format - single digit day",
			dateKey: "09-4",
			wantErr: "expected MM-DD format",
		},
		{
			name:    "invalid month",
			dateKey: "13-01",
			wantErr: "expected MM-DD format",
		},
		{
			name:    "invalid day",
			dateKey: "01-32",
			wantErr: "expected MM-DD format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &PublishRequest{
				DateKey:   tt.dateKey,
				Platforms: []string{"bluesky"},
				DryRun:    false,
			}

			response, err := service.PublishAnniversaryPosts(context.Background(), req)

			assert.Error(t, err)
			assert.Nil(t, response)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestPublishAnniversaryPosts_GeneratorErrors(t *testing.T) {
	ctx := context.Background()

	// Setup test data
	posts := []*domain.Post{
		createTestPost("https://example.com/post1", "2020-09-14"),
	}

	tests := []struct {
		name    string
		genErr  error
		wantErr bool
	}{
		{
			name:    "generation API error",
			genErr:  errors.New("API rate limit exceeded"),
			wantErr: true,
		},
		{
			name:    "generation timeout",
			genErr:  errors.New("context deadline exceeded"),
			wantErr: true,
		},
		{
			name:    "invalid prompt",
			genErr:  ports.ErrInvalidPrompt,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mocks
			repo := &MockPostRepository{}
			repo.On("GetPostForDate", ctx, "09-14").Return(posts[0], nil)

			gen := &MockContentGenerator{}
			gen.On("GenerateContent", ctx, mock.AnythingOfType("*ports.GenerationRequest")).Return(nil, tt.genErr)

			publishers := map[string]ports.SocialPublisher{
				"bluesky": NewMockSocialPublisher("bluesky"),
			}

			service := createTestPublishService(repo, gen, publishers)

			// Execute
			req := &PublishRequest{
				DateKey:   "09-14",
				Platforms: []string{"bluesky"},
				DryRun:    false,
			}

			_, err := service.PublishAnniversaryPosts(ctx, req)

			// Should continue gracefully with generator errors
			require.Error(t, err)
			assert.Contains(t, err.Error(), "failed to generate content")

			// Verify mocks
			repo.AssertExpectations(t)
			gen.AssertExpectations(t)
		})
	}
}

func TestPublishAnniversaryPosts_PublisherErrors(t *testing.T) {
	ctx := context.Background()

	// Setup test data
	posts := []*domain.Post{
		createTestPost("https://example.com/post1", "2020-09-14"),
	}

	tests := []struct {
		name      string
		pubErr    error
		wantErr   bool
		wantCount int
	}{
		{
			name:      "publish API error",
			pubErr:    errors.New("authentication failed"),
			wantErr:   false, // Should continue gracefully
			wantCount: 0,     // No successful publishes
		},
		{
			name:      "rate limit exceeded",
			pubErr:    ports.ErrRateLimitExceeded,
			wantErr:   false, // Should continue gracefully
			wantCount: 0,     // No successful publishes
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mocks
			repo := &MockPostRepository{}
			repo.On("GetPostForDate", ctx, "09-14").Return(posts[0], nil)

			gen := &MockContentGenerator{}
			blueskycontent := createTestGeneratedContent("Test content", "bluesky")
			gen.On("GenerateContent", ctx, mock.AnythingOfType("*ports.GenerationRequest")).Return(blueskycontent, nil)

			blueskyPub := NewMockSocialPublisher("bluesky")
			blueskyPub.On("ValidateContent", ctx, blueskycontent).Return(nil)
			blueskyPub.On("Publish", ctx, blueskycontent).Return(nil, tt.pubErr)

			publishers := map[string]ports.SocialPublisher{
				"bluesky": blueskyPub,
			}

			service := createTestPublishService(repo, gen, publishers)

			// Execute
			req := &PublishRequest{
				DateKey:   "09-14",
				Platforms: []string{"bluesky"},
				DryRun:    false,
			}

			response, err := service.PublishAnniversaryPosts(ctx, req)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantCount, response.Result.SuccessCount())
				assert.True(t, response.Result.HasFailures())
			}

			// Verify mocks
			repo.AssertExpectations(t)
			gen.AssertExpectations(t)
			blueskyPub.AssertExpectations(t)
		})
	}
}

func TestPublishAnniversaryPosts_ContextCancellation(t *testing.T) {
	// Setup test data
	posts := []*domain.Post{
		createTestPost("https://example.com/post1", "2020-09-14"),
	}

	// Setup mocks
	repo := &MockPostRepository{}
	repo.On("GetPostForDate", mock.AnythingOfType("*context.cancelCtx"), "09-14").Return(posts[0], nil)

	gen := &MockContentGenerator{}
	publishers := map[string]ports.SocialPublisher{
		"bluesky": NewMockSocialPublisher("bluesky"),
	}

	service := createTestPublishService(repo, gen, publishers)

	// Execute with cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	req := &PublishRequest{
		DateKey:   "09-14",
		Platforms: []string{"bluesky"},
		DryRun:    false,
	}

	response, err := service.PublishAnniversaryPosts(ctx, req)

	// Assertions
	require.Error(t, err)
	assert.Nil(t, response)
	assert.Contains(t, err.Error(), "operation was cancelled")

	// Verify mocks
	repo.AssertExpectations(t)
}

func TestResolvePlatforms(t *testing.T) {
	blueskyPub := NewMockSocialPublisher("bluesky")
	mastodonPub := NewMockSocialPublisher("mastodon")

	publishers := map[string]ports.SocialPublisher{
		"bluesky":  blueskyPub,
		"mastodon": mastodonPub,
	}

	service := createTestPublishService(nil, nil, publishers)

	tests := []struct {
		name      string
		platforms []string
		expected  []string
	}{
		{
			name:      "single platform",
			platforms: []string{"bluesky"},
			expected:  []string{"bluesky"},
		},
		{
			name:      "multiple platforms",
			platforms: []string{"bluesky", "mastodon"},
			expected:  []string{"bluesky", "mastodon"},
		},
		{
			name:      "all platforms",
			platforms: []string{"all"},
			expected:  []string{"bluesky", "mastodon"},
		},
		{
			name:      "mixed with all",
			platforms: []string{"bluesky", "all"},
			expected:  []string{"bluesky", "mastodon"},
		},
		{
			name:      "unavailable platform filtered out",
			platforms: []string{"bluesky", "twitter"},
			expected:  []string{"bluesky"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := service.resolvePlatforms(tt.platforms)

			require.NoError(t, err)
			assert.ElementsMatch(t, tt.expected, result)
		})
	}
}

func TestValidateRequest(t *testing.T) {
	service := createTestPublishService(nil, nil, nil)

	tests := []struct {
		name    string
		req     *PublishRequest
		wantErr string
	}{
		{
			name:    "nil request",
			req:     nil,
			wantErr: "request cannot be nil",
		},
		{
			name: "valid request",
			req: &PublishRequest{
				DateKey:   "09-14",
				Platforms: []string{"bluesky"},
				DryRun:    false,
			},
			wantErr: "",
		},
		{
			name: "empty platforms",
			req: &PublishRequest{
				DateKey:   "09-14",
				Platforms: []string{},
				DryRun:    false,
			},
			wantErr: "at least one platform must be specified",
		},
		{
			name: "invalid platform",
			req: &PublishRequest{
				DateKey:   "09-14",
				Platforms: []string{"invalid"},
				DryRun:    false,
			},
			wantErr: "invalid platform",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := service.validateRequest(tt.req)

			if tt.wantErr == "" {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			}
		})
	}
}

func TestCreateMockContent(t *testing.T) {
	posts := []*domain.Post{
		createTestPost("https://example.com/post1", "2020-09-14"),
	}

	service := createTestPublishService(nil, nil, nil)

	// Test Bluesky mock content
	content := service.createMockContent("bluesky", posts[0], "09-14")

	assert.NotNil(t, content)
	assert.Equal(t, "bluesky", content.Platform())
	assert.Contains(t, content.Text(), "Anniversary post")
	assert.Contains(t, content.Text(), "09-14")
	assert.True(t, len(content.Text()) <= domain.BlueskyCharacterLimit)
	assert.NotEmpty(t, content.Hashtags())

	// Test Mastodon mock content
	content = service.createMockContent("mastodon", posts[0], "09-14")

	assert.Equal(t, "mastodon", content.Platform())
	assert.True(t, len(content.Text()) <= domain.MastodonCharacterLimit)
}
