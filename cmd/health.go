package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/jbspeakr/urbanartcore-bot/internal/app"
	"github.com/jbspeakr/urbanartcore-bot/internal/infrastructure/prompts"
)

// healthCmd represents the health command
var healthCmd = &cobra.Command{
	Use:   "health",
	Short: "Perform health checks on application dependencies",
	Long: `Validate configuration, data files, and prompt templates to ensure the application
is properly configured and can run successfully.

This command checks:
  - Configuration loading from environment variables
  - Data file existence and readability
  - Prompt template files existence and validity

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
			slog.String("data_path", config.DataPath),
			slog.String("prompts_path", config.PromptsPath),
			slog.String("mistral_model", config.MistralModel))
	}

	// Check 2: Data file existence and readability
	if config != nil {
		logger.Info("Health check: Data file")
		dataPath := config.DataPath
		if dataPath == "" {
			dataPath = "./data/posts_by_date.json"
		}

		// Check file exists
		info, err := os.Stat(dataPath)
		if err != nil {
			logger.Error("Data file check failed",
				slog.String("path", dataPath),
				slog.String("error", err.Error()))
			allChecksPass = false
		} else if info.IsDir() {
			logger.Error("Data file check failed",
				slog.String("path", dataPath),
				slog.String("error", "path is a directory, not a file"))
			allChecksPass = false
		} else {
			// Check file is readable
			file, err := os.Open(dataPath)
			if err != nil {
				logger.Error("Data file check failed",
					slog.String("path", dataPath),
					slog.String("error", fmt.Sprintf("cannot read file: %v", err)))
				allChecksPass = false
			} else {
				if err := file.Close(); err != nil {
					logger.Warn("Failed to close data file",
						slog.String("path", dataPath),
						slog.String("error", err.Error()))
				}
				logger.Info("Data file check passed",
					slog.String("path", dataPath),
					slog.Int64("size_bytes", info.Size()))
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

	// Final summary
	if allChecksPass {
		logger.Info("All health checks passed")
		return nil
	}

	logger.Error("Some health checks failed")
	return fmt.Errorf("health checks failed")
}
