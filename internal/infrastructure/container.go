package infrastructure

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"codeberg.org/brennenstuhl/uac-socializer/internal/adapters/repository"
	"codeberg.org/brennenstuhl/uac-socializer/internal/app"
	"codeberg.org/brennenstuhl/uac-socializer/internal/domain"
	"codeberg.org/brennenstuhl/uac-socializer/internal/ports"
)

// Placeholder repository implementation
type placeholderRepository struct{}

func (r *placeholderRepository) GetPostForDate(ctx context.Context, dateKey string) (*domain.Post, error) {
	return nil, fmt.Errorf("not implemented: GetPostForDate will be available in Phase 3")
}

func (r *placeholderRepository) ListAvailableDates(ctx context.Context) ([]string, error) {
	return nil, fmt.Errorf("not implemented: ListAvailableDates will be available in Phase 2")
}

func (r *placeholderRepository) GetPostCount(ctx context.Context) (int, error) {
	return 0, fmt.Errorf("not implemented: GetPostCount will be available in Phase 2")
}

func (r *placeholderRepository) ValidateDataSource(ctx context.Context) error {
	return fmt.Errorf("not implemented: ValidateDataSource will be available in Phase 2")
}

// Placeholder publisher implementation
type placeholderPublisher struct {
	platform string
}

func newPlaceholderPublisher(platform string) *placeholderPublisher {
	return &placeholderPublisher{platform: platform}
}

func (p *placeholderPublisher) Platform() string {
	return p.platform
}

func (p *placeholderPublisher) Publish(ctx context.Context, content *domain.GeneratedContent) (*domain.PlatformResult, error) {
	return nil, fmt.Errorf("not implemented: %s publisher will be available in Phase 4", p.platform)
}

func (p *placeholderPublisher) ValidateContent(ctx context.Context, content *domain.GeneratedContent) error {
	return fmt.Errorf("not implemented: %s content validation will be available in Phase 4", p.platform)
}

func (p *placeholderPublisher) GetCharacterLimit() int {
	switch p.platform {
	case domain.PlatformBluesky:
		return domain.BlueskyCharacterLimit
	case domain.PlatformMastodon:
		return domain.MastodonCharacterLimit
	default:
		return 280
	}
}

func (p *placeholderPublisher) SupportsHashtags() bool {
	return true // Most platforms support hashtags
}

func (p *placeholderPublisher) SupportsMedia() bool {
	return true // Most platforms support media
}

func (p *placeholderPublisher) TestConnection(ctx context.Context) error {
	return fmt.Errorf("not implemented: %s connection test will be available in Phase 4", p.platform)
}

// Placeholder content generator implementation
type placeholderGenerator struct{}

func (g *placeholderGenerator) GenerateContent(ctx context.Context, req *ports.GenerationRequest) (*domain.GeneratedContent, error) {
	return nil, fmt.Errorf("not implemented: GenerateContent will be available in Phase 5")
}

func (g *placeholderGenerator) GenerateForAllPlatforms(ctx context.Context, posts []*domain.Post, dateKey string) (map[string]*domain.GeneratedContent, error) {
	return nil, fmt.Errorf("not implemented: GenerateForAllPlatforms will be available in Phase 5")
}

func (g *placeholderGenerator) ValidatePrompt(ctx context.Context, platform string, prompt string) error {
	return fmt.Errorf("not implemented: ValidatePrompt will be available in Phase 5")
}

func (g *placeholderGenerator) GetSupportedModels(ctx context.Context) ([]string, error) {
	return nil, fmt.Errorf("not implemented: GetSupportedModels will be available in Phase 5")
}

func (g *placeholderGenerator) TestConnection(ctx context.Context) error {
	return fmt.Errorf("not implemented: TestConnection will be available in Phase 5")
}

// Container holds all application dependencies.
// Logger is not included as a field - use slog.Default() globally after initialization.
type Container struct {
	Config            *app.Config
	PostRepository    ports.PostRepository
	BlueskyPublisher  ports.SocialPublisher
	MastodonPublisher ports.SocialPublisher
	ContentGenerator  ports.ContentGenerator
}

// NewContainer creates and initializes a new dependency injection container.
// Logger must be initialized separately via InitializeLogger before creating the container.
func NewContainer(ctx context.Context, config *app.Config) (*Container, error) {
	const op = "NewContainer"

	if config == nil {
		return nil, fmt.Errorf("%s: config cannot be nil", op)
	}

	// Validate configuration first
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("%s: invalid configuration: %w", op, err)
	}

	logger := slog.Default()

	logger.Debug("Initializing dependency container",
		slog.String("operation", op),
		slog.Any("config", config.SanitizeForLogging()),
	)

	// Initialize repository with real JSONFileRepository implementation
	repo, err := repository.NewJSONFileRepository(config.DataPath)
	if err != nil {
		return nil, fmt.Errorf("%s: failed to initialize repository: %w", op, err)
	}

	logger.Debug("Initialized post repository",
		slog.String("type", "JSONFileRepository"),
		slog.String("dataPath", config.DataPath))

	// Initialize social publishers (placeholders for Phase 4)
	blueskyPublisher := newPlaceholderPublisher(domain.PlatformBluesky)
	mastodonPublisher := newPlaceholderPublisher(domain.PlatformMastodon)
	logger.Debug("Initialized social publishers",
		slog.String("bluesky", "placeholder"),
		slog.String("mastodon", "placeholder"),
	)

	// Initialize content generator (placeholder for Phase 5)
	contentGenerator := &placeholderGenerator{}
	logger.Debug("Initialized content generator", slog.String("type", "placeholder"))

	container := &Container{
		Config:            config,
		PostRepository:    repo,
		BlueskyPublisher:  blueskyPublisher,
		MastodonPublisher: mastodonPublisher,
		ContentGenerator:  contentGenerator,
	}

	logger.Info("Dependency container initialized successfully",
		slog.String("operation", op),
		slog.String("log_level", config.LogLevel),
		slog.String("data_path", config.DataPath),
	)

	return container, nil
}

// Validate performs health checks on all container components.
func (c *Container) Validate(ctx context.Context) error {
	const op = "Container.Validate"
	logger := slog.Default()

	logger.Debug("Starting container validation", slog.String("operation", op))

	// Verify config is valid
	if err := c.Config.Validate(); err != nil {
		return fmt.Errorf("%s: configuration invalid: %w", op, err)
	}

	// Verify repository is accessible (this will fail with placeholder)
	if err := c.PostRepository.ValidateDataSource(ctx); err != nil {
		// For placeholders, we expect this to fail, so we'll log a warning instead
		logger.Warn("Repository validation failed (expected for placeholder)",
			slog.String("operation", op),
			slog.String("error", err.Error()),
		)
	}

	// Test publisher connections (these will fail with placeholders)
	if err := c.BlueskyPublisher.TestConnection(ctx); err != nil {
		logger.Warn("Bluesky publisher connection test failed (expected for placeholder)",
			slog.String("operation", op),
			slog.String("error", err.Error()),
		)
	}

	if err := c.MastodonPublisher.TestConnection(ctx); err != nil {
		logger.Warn("Mastodon publisher connection test failed (expected for placeholder)",
			slog.String("operation", op),
			slog.String("error", err.Error()),
		)
	}

	// Test content generator connection (this will fail with placeholder)
	if err := c.ContentGenerator.TestConnection(ctx); err != nil {
		logger.Warn("Content generator connection test failed (expected for placeholder)",
			slog.String("operation", op),
			slog.String("error", err.Error()),
		)
	}

	logger.Debug("Container validation completed", slog.String("operation", op))
	return nil
}

// Close releases any resources held by the container.
func (c *Container) Close() error {
	const op = "Container.Close"
	slog.Default().Debug("Closing dependency container", slog.String("operation", op))

	// Future cleanup will happen here:
	// - Close database connections
	// - Stop background workers
	// - Release file handles
	// - Clean up temporary resources

	return nil
}

// GetRepository returns the configured post repository.
func (c *Container) GetRepository() ports.PostRepository {
	return c.PostRepository
}

// GetPublisher returns the publisher for the specified platform.
func (c *Container) GetPublisher(platform string) (ports.SocialPublisher, error) {
	platform = strings.ToLower(strings.TrimSpace(platform))

	switch platform {
	case domain.PlatformBluesky:
		return c.BlueskyPublisher, nil
	case domain.PlatformMastodon:
		return c.MastodonPublisher, nil
	default:
		return nil, fmt.Errorf("unsupported platform: %s", platform)
	}
}

// GetContentGenerator returns the configured content generator.
func (c *Container) GetContentGenerator() ports.ContentGenerator {
	return c.ContentGenerator
}

// GetConfig returns the application configuration.
func (c *Container) GetConfig() *app.Config {
	return c.Config
}

// GetPublishService creates and returns a new PublishService with all dependencies wired.
func (c *Container) GetPublishService() *app.PublishService {
	publishers := map[string]ports.SocialPublisher{
		domain.PlatformBluesky:  c.BlueskyPublisher,
		domain.PlatformMastodon: c.MastodonPublisher,
	}

	return app.NewPublishService(
		c.PostRepository,
		c.ContentGenerator,
		publishers,
		slog.Default(),
	)
}
