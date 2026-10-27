package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/jbspeakr/urbanartcore-bot/internal/adapters/generator"
	"github.com/jbspeakr/urbanartcore-bot/internal/adapters/site"
	"github.com/jbspeakr/urbanartcore-bot/internal/app"
	"github.com/jbspeakr/urbanartcore-bot/internal/infrastructure/prompts"
)

// healthCmd represents the health command
var healthCmd = &cobra.Command{
	Use:   "health",
	Short: "Perform health checks on application dependencies",
	Long: `Validate configuration, the post catalogue, and prompt templates to ensure the application
is properly configured and can run successfully.

This command checks:
  - Configuration loading from environment variables
  - Post catalogue from the site, or the built-in snapshot when the site is down
  - Prompt template files existence and validity
  - Mistral model availability (when UAC_MISTRAL_API_KEY is set)

Exit codes:
  0 - All health checks passed
  1 - One or more health checks failed

Example:
  uac-socializer health`,
	RunE: runHealthCheck,
}

func init() {
	rootCmd.AddCommand(healthCmd)
}

// runHealthCheck executes all health checks and reports results.
func runHealthCheck(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Initialize logger for health check output
	opts := &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}
	handler := slog.NewJSONHandler(os.Stdout, opts)
	slog.SetDefault(slog.New(handler))

	logger := slog.Default()
	logger.Info("Starting health checks")

	allChecksPass := true

	// Check 1: Configuration loading
	logger.Info("Health check: Configuration")
	config, err := app.LoadConfig()
	if err != nil {
		logger.Error("Configuration check failed",
			slog.String("error", err.Error()))
		allChecksPass = false
	} else {
		logger.Info("Configuration check passed",
			slog.String("site_base_url", config.SiteBaseURL),
			slog.String("prompts_path", config.PromptsPath),
			slog.String("mistral_model", config.MistralModel))
	}

	// Check 2: Post catalogue, live from the site or from the built-in snapshot
	if config != nil {
		logger.Info("Health check: Post catalogue")
		client, err := site.NewClient(config.SiteBaseURL)
		if err == nil {
			err = client.ValidateDataSource(ctx)
		}
		if err != nil {
			logger.Error("Post catalogue check failed",
				slog.String("site_base_url", config.SiteBaseURL),
				slog.String("error", err.Error()))
			allChecksPass = false
		} else {
			count, _ := client.GetPostCount(ctx)
			attrs := []any{
				slog.String("site_base_url", config.SiteBaseURL),
				slog.String("source", client.Source()),
				slog.Int("posts", count),
			}
			if client.Source() == site.SourceSnapshot {
				logger.Warn("Post catalogue check passed on the built-in snapshot; the site could not be reached", attrs...)
			} else {
				logger.Info("Post catalogue check passed", attrs...)
			}
		}
	}

	// Check 3: Prompt templates
	if config != nil {
		logger.Info("Health check: Prompt templates")
		promptsPath := config.PromptsPath
		if promptsPath == "" {
			promptsPath = "./internal/infrastructure/prompts"
		}

		// Check prompts directory exists
		info, err := os.Stat(promptsPath)
		if err != nil {
			logger.Error("Prompts directory check failed",
				slog.String("path", promptsPath),
				slog.String("error", err.Error()))
			allChecksPass = false
		} else if !info.IsDir() {
			logger.Error("Prompts directory check failed",
				slog.String("path", promptsPath),
				slog.String("error", "path is not a directory"))
			allChecksPass = false
		} else {
			// Try to initialize prompt manager
			pm, err := prompts.NewPromptManager(promptsPath)
			if err != nil {
				logger.Error("Prompt templates check failed",
					slog.String("path", promptsPath),
					slog.String("error", err.Error()))
				allChecksPass = false
			} else {
				// Validate individual prompt files
				templates, err := pm.ListPrompts(ctx)
				if err != nil {
					logger.Error("Prompt templates listing failed",
						slog.String("error", err.Error()))
					allChecksPass = false
				} else {
					// Check each platform has a valid template
					requiredPlatforms := []string{"bluesky", "mastodon"}
					for _, platform := range requiredPlatforms {
						templatePath := filepath.Join(promptsPath, platform+".txt")
						if _, exists := templates[platform]; !exists {
							logger.Error("Prompt template missing",
								slog.String("platform", platform),
								slog.String("expected_path", templatePath))
							allChecksPass = false
						} else {
							// Validate template
							if err := pm.ValidateTemplate(ctx, templates[platform]); err != nil {
								logger.Error("Prompt template validation failed",
									slog.String("platform", platform),
									slog.String("path", templatePath),
									slog.String("error", err.Error()))
								allChecksPass = false
							} else {
								logger.Info("Prompt template check passed",
									slog.String("platform", platform),
									slog.String("path", templatePath))
							}
						}
					}
				}
			}
		}
	}

	// Check 4: Mistral model availability
	if config != nil && config.MistralAPIKey != "" {
		logger.Info("Health check: Mistral model")
		pm, err := prompts.NewPromptManager(config.PromptsPath)
		if err != nil {
			logger.Error("Mistral model check skipped",
				slog.String("error", err.Error()))
			allChecksPass = false
		} else if gen, err := generator.NewMistralAdapter(config.GetMistralConfig(), pm); err != nil {
			logger.Error("Mistral model check failed",
				slog.String("error", err.Error()))
			allChecksPass = false
		} else if err := gen.TestConnection(ctx); err != nil {
			logger.Error("Mistral model check failed",
				slog.String("model", config.MistralModel),
				slog.String("error", err.Error()))
			allChecksPass = false
		} else {
			logger.Info("Mistral model check passed",
				slog.String("model", config.MistralModel))
		}
	}

	// Final summary
	if allChecksPass {
		logger.Info("All health checks passed")
		return nil
	}

	logger.Error("Some health checks failed")
	return fmt.Errorf("health checks failed")
}
