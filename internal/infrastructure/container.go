package infrastructure

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jbspeakr/urbanartcore-bot/internal/adapters/generator"
	"github.com/jbspeakr/urbanartcore-bot/internal/adapters/ledger"
	"github.com/jbspeakr/urbanartcore-bot/internal/adapters/publisher"
	"github.com/jbspeakr/urbanartcore-bot/internal/adapters/site"
	"github.com/jbspeakr/urbanartcore-bot/internal/app"
	"github.com/jbspeakr/urbanartcore-bot/internal/domain"
	"github.com/jbspeakr/urbanartcore-bot/internal/infrastructure/prompts"
	"github.com/jbspeakr/urbanartcore-bot/internal/ports"
)

// Container holds all application dependencies.
// Logger is not included as a field - use slog.Default() globally after initialization.
type Container struct {
	Config            *app.Config
	PostRepository    ports.PostRepository
	PostReader        ports.PostReader
	BlueskyPublisher  ports.SocialPublisher
	MastodonPublisher ports.SocialPublisher
	ContentGenerator  ports.ContentGenerator
	Critic            ports.Critic         // nil when no critic model is configured
	Describer         ports.ImageDescriber // nil when no vision model is configured
	Archive           ports.Archive
	Ledger            ports.Ledger // nil when no ledger bucket is configured
	PromptManager     ports.PromptManager
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
		slog.Any("config", config.SanitizeForLogging()),
	)

	// The site's JSON is both the catalogue and the source of each article;
	// the catalogue is fetched on first use
	siteClient, err := site.NewClient(config.SiteBaseURL)
	if err != nil {
		return nil, fmt.Errorf("%s: failed to initialize site client: %w", op, err)
	}

	logger.Debug("Initialized site client",
		slog.String("type", "site.Client"),
		slog.String("siteBaseURL", config.SiteBaseURL))

	// Initialize social publishers with real adapters (Phase 4)
	blueskyPublisher, err := publisher.NewBlueskyAdapter(config.BlueskyHandle, config.BlueskyPassword)
	if err != nil {
		return nil, fmt.Errorf("%s: failed to initialize Bluesky publisher: %w", op, err)
	}
	logger.Debug("Initialized Bluesky publisher",
		slog.String("type", "BlueskyAdapter"),
		slog.String("handle", config.BlueskyHandle))

	mastodonPublisher, err := publisher.NewMastodonAdapter(config.MastodonServer, config.MastodonAccessToken)
	if err != nil {
		return nil, fmt.Errorf("%s: failed to initialize Mastodon publisher: %w", op, err)
	}
	logger.Debug("Initialized Mastodon publisher",
		slog.String("type", "MastodonAdapter"),
		slog.String("server", config.MastodonServer))

	// Initialize prompt manager with real implementation
	promptManager, err := prompts.NewPromptManager(config.PromptsPath)
	if err != nil {
		return nil, fmt.Errorf("%s: failed to initialize prompt manager: %w", op, err)
	}
	logger.Debug("Initialized prompt manager",
		slog.String("type", "PromptManager"),
		slog.String("prompts_path", config.PromptsPath))

	// Initialize content generator with real MistralAdapter (Phase 5)
	contentGenerator, err := generator.NewMistralAdapter(config.GetMistralConfig(), promptManager)
	if err != nil {
		return nil, fmt.Errorf("%s: failed to initialize content generator: %w", op, err)
	}
	logger.Debug("Initialized content generator",
		slog.String("type", "MistralAdapter"),
		slog.String("model", config.MistralModel))

	// The critic checks every draft against its post before it is published
	var critic ports.Critic
	if config.CriticModel != "" {
		mistralCritic, err := generator.NewMistralCritic(config.MistralAPIKey, config.CriticModel, "", config.AITimeout)
		if err != nil {
			return nil, fmt.Errorf("%s: failed to initialize critic: %w", op, err)
		}
		critic = mistralCritic
		logger.Debug("Initialized critic",
			slog.String("type", "MistralCritic"),
			slog.String("model", config.CriticModel))
	}

	// The describer writes the alt text of cover photos
	var describer ports.ImageDescriber
	if config.VisionModel != "" {
		mistralDescriber, err := generator.NewMistralDescriber(config.MistralAPIKey, config.VisionModel, "", config.AITimeout)
		if err != nil {
			return nil, fmt.Errorf("%s: failed to initialize image describer: %w", op, err)
		}
		describer = mistralDescriber
		logger.Debug("Initialized image describer",
			slog.String("type", "MistralDescriber"),
			slog.String("model", config.VisionModel))
	}

	// The ledger keeps reruns from posting twice and posts from repeating
	var publishLedger ports.Ledger
	if config.HasLedger() {
		objectStorage, err := ledger.NewObjectStorage(ledger.Config{
			Endpoint:  config.LedgerEndpoint,
			Region:    config.LedgerRegion,
			Bucket:    config.LedgerBucket,
			AccessKey: config.LedgerAccessKey,
			SecretKey: config.LedgerSecretKey,
		})
		if err != nil {
			return nil, fmt.Errorf("%s: failed to initialize ledger: %w", op, err)
		}
		publishLedger = objectStorage
		logger.Debug("Initialized ledger",
			slog.String("type", "ObjectStorage"),
			slog.String("bucket", config.LedgerBucket))
	} else {
		logger.Warn("No ledger bucket configured: reruns will post again and posts may repeat")
	}

	container := &Container{
		Config:            config,
		PostRepository:    siteClient,
		PostReader:        siteClient,
		Archive:           siteClient,
		BlueskyPublisher:  blueskyPublisher,
		MastodonPublisher: mastodonPublisher,
		ContentGenerator:  contentGenerator,
		Critic:            critic,
		Describer:         describer,
		Ledger:            publishLedger,
		PromptManager:     promptManager,
	}

	logger.Info("Dependency container initialized successfully",
		slog.String("site_base_url", config.SiteBaseURL),
	)

	return container, nil
}

// Validate performs health checks on all container components.
func (c *Container) Validate(ctx context.Context) error {
	const op = "Container.Validate"
	logger := slog.Default()

	logger.Debug("Starting container validation")

	// Verify config is valid
	if err := c.Config.Validate(); err != nil {
		return fmt.Errorf("%s: configuration invalid: %w", op, err)
	}

	// Verify repository is accessible
	if err := c.PostRepository.ValidateDataSource(ctx); err != nil {
		logger.Warn("Repository validation failed",
			slog.String("error", err.Error()),
		)
	} else {
		logger.Debug("Repository validation successful")
	}

	// Test publisher connections
	if err := c.BlueskyPublisher.TestConnection(ctx); err != nil {
		logger.Warn("Bluesky publisher connection test failed",
			slog.String("error", err.Error()),
		)
	} else {
		logger.Debug("Bluesky publisher connection test successful")
	}

	if err := c.MastodonPublisher.TestConnection(ctx); err != nil {
		logger.Warn("Mastodon publisher connection test failed",
			slog.String("error", err.Error()),
		)
	} else {
		logger.Debug("Mastodon publisher connection test successful")
	}

	// Test content generator connection
	if err := c.ContentGenerator.TestConnection(ctx); err != nil {
		logger.Warn("Content generator connection test failed",
			slog.String("error", err.Error()),
		)
	} else {
		logger.Debug("Content generator connection test successful")
	}

	// Test prompt manager by attempting to list prompts
	if prompts, err := c.PromptManager.ListPrompts(ctx); err != nil {
		logger.Warn("Prompt manager list prompts test failed",
			slog.String("error", err.Error()),
		)
	} else {
		logger.Debug("Prompt manager validation successful",
			slog.Int("prompts_available", len(prompts)),
		)
	}

	logger.Debug("Container validation completed")
	return nil
}

// Close releases any resources held by the container.
func (c *Container) Close() error {
	slog.Default().Debug("Closing dependency container")

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

// GetPromptManager returns the configured prompt manager.
func (c *Container) GetPromptManager() ports.PromptManager {
	return c.PromptManager
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
	).WithPostReader(c.PostReader).WithCritic(c.Critic).WithLedger(c.Ledger).WithImageDescriber(c.Describer).WithArchive(c.Archive)
}
