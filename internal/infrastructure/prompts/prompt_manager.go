package prompts

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/jbspeakr/urbanartcore-bot/internal/domain"
	"github.com/jbspeakr/urbanartcore-bot/internal/ports"
)

// PromptManagerError represents errors from the prompt manager.
type PromptManagerError struct {
	Op       string // Operation that caused the error
	Platform string // Platform (if applicable)
	Err      error  // Underlying error
}

func (e PromptManagerError) Error() string {
	if e.Platform != "" {
		return fmt.Sprintf("prompt_manager[%s].%s: %v", e.Platform, e.Op, e.Err)
	}
	return fmt.Sprintf("prompt_manager.%s: %v", e.Op, e.Err)
}

func (e PromptManagerError) Unwrap() error {
	return e.Err
}

// Template variable pattern for validation and substitution
// Matches {VARIABLE_NAME} where name can contain uppercase letters, underscores, and digits
var templateVarPattern = regexp.MustCompile(`{[A-Z_0-9]+}`)

// PromptManager implements the ports.PromptManager interface.
// It loads, caches, and renders prompt templates for AI content generation.
type PromptManager struct {
	promptsDir string            // Directory containing prompt template files
	templates  map[string]string // Cached templates by platform
	mu         sync.RWMutex      // Protects concurrent access to templates
}

// NewPromptManager creates a new PromptManager and loads all templates.
// It validates the directory exists and loads templates during initialization.
func NewPromptManager(promptsDir string) (*PromptManager, error) {

	if promptsDir == "" {
		return nil, PromptManagerError{

			Err: fmt.Errorf("prompts directory path cannot be empty"),
		}
	}

	// Verify directory exists
	info, err := os.Stat(promptsDir)
	if os.IsNotExist(err) {
		return nil, PromptManagerError{

			Err: fmt.Errorf("prompts directory does not exist: %s", promptsDir),
		}
	}
	if err != nil {
		return nil, PromptManagerError{

			Err: fmt.Errorf("failed to access prompts directory %s: %w", promptsDir, err),
		}
	}
	if !info.IsDir() {
		return nil, PromptManagerError{

			Err: fmt.Errorf("prompts path is not a directory: %s", promptsDir),
		}
	}

	pm := &PromptManager{
		promptsDir: promptsDir,
		templates:  make(map[string]string),
	}

	// Load all templates
	if err := pm.loadTemplates(); err != nil {
		return nil, PromptManagerError{Err: err}
	}

	slog.Default().Info("Prompt manager initialized",
		slog.String("prompts_dir", promptsDir),
		slog.Int("templates_loaded", len(pm.templates)))

	return pm, nil
}

// loadTemplates loads all prompt template files from the prompts directory.
func (pm *PromptManager) loadTemplates() error {
	// Load templates for all supported platforms
	platforms := []string{domain.PlatformBluesky, domain.PlatformMastodon}

	for _, platform := range platforms {
		templatePath := filepath.Join(pm.promptsDir, platform+".txt")

		// Check if template file exists
		if _, err := os.Stat(templatePath); os.IsNotExist(err) {
			return fmt.Errorf("template file not found for platform %s: %s", platform, templatePath)
		}

		// Read template file
		content, err := os.ReadFile(templatePath)
		if err != nil {
			return fmt.Errorf("failed to read template file %s: %w", templatePath, err)
		}

		template := string(content)

		// Validate template
		if strings.TrimSpace(template) == "" {
			return fmt.Errorf("template file is empty: %s", templatePath)
		}

		// Store template
		pm.templates[platform] = template

		slog.Default().Debug("Loaded prompt template",
			slog.String("platform", platform),
			slog.String("file", templatePath),
			slog.Int("length", len(template)))
	}

	return nil
}

// GetPrompt retrieves the prompt template for a specific platform.
func (pm *PromptManager) GetPrompt(ctx context.Context, platform string) (string, error) {

	if pm == nil {
		return "", PromptManagerError{

			Platform: platform,
			Err:      fmt.Errorf("prompt manager is nil"),
		}
	}

	// Validate platform
	platform = strings.ToLower(strings.TrimSpace(platform))
	if !domain.IsValidPlatform(platform) {
		return "", PromptManagerError{

			Platform: platform,
			Err:      fmt.Errorf("%w: %q", domain.ErrInvalidPlatform, platform),
		}
	}

	// Check context cancellation
	select {
	case <-ctx.Done():
		return "", PromptManagerError{Platform: platform, Err: ctx.Err()}
	default:
	}

	pm.mu.RLock()
	defer pm.mu.RUnlock()

	template, exists := pm.templates[platform]
	if !exists {
		return "", PromptManagerError{

			Platform: platform,
			Err:      fmt.Errorf("template not found for platform: %s", platform),
		}
	}

	return template, nil
}

// RenderPrompt processes a template with variables from the generation request.
// Returns the final prompt ready to send to the AI service.
func (pm *PromptManager) RenderPrompt(ctx context.Context, platform string, req *ports.GenerationRequest) (string, error) {

	if pm == nil {
		return "", PromptManagerError{

			Platform: platform,
			Err:      fmt.Errorf("prompt manager is nil"),
		}
	}

	if req == nil {
		return "", PromptManagerError{

			Platform: platform,
			Err:      fmt.Errorf("generation request cannot be nil"),
		}
	}

	// Get base template
	template, err := pm.GetPrompt(ctx, platform)
	if err != nil {
		return "", PromptManagerError{Platform: platform, Err: err}
	}

	// Build variable map from request
	vars := pm.buildVariableMap(req)

	// Perform variable substitution
	rendered := template
	for key, value := range vars {
		placeholder := fmt.Sprintf("{%s}", key)
		rendered = strings.ReplaceAll(rendered, placeholder, value)
	}

	// Check for unresolved variables
	if unresolved := templateVarPattern.FindAllString(rendered, -1); len(unresolved) > 0 {
		slog.Default().Warn("Template contains unresolved variables",
			slog.String("platform", platform),
			slog.Any("unresolved", unresolved))
	}

	slog.Default().Debug("Rendered prompt template",
		slog.String("platform", platform),
		slog.Int("original_length", len(template)),
		slog.Int("rendered_length", len(rendered)),
		slog.String("date_key", req.DateKey))

	return rendered, nil
}

// buildVariableMap creates a map of template variables from the generation request.
func (pm *PromptManager) buildVariableMap(req *ports.GenerationRequest) map[string]string {
	vars := make(map[string]string)

	// Calculate anniversary years from first post
	if len(req.Posts) > 0 {
		post := req.Posts[0]
		years := time.Now().Year() - post.Date().Year()
		vars["ANNIVERSARY_YEARS"] = fmt.Sprintf("%d", years)

		// Add post URI
		vars["POST_URI"] = post.URI()
	}

	// Add platform-specific constraints
	switch req.Platform {
	case domain.PlatformBluesky:
		vars["PLATFORM_CONSTRAINTS"] = "300 bytes (UTF-8), concise and conversational"
		vars["TONE"] = "casual, community-focused, enthusiastic"
		vars["COMMUNITY_CONTEXT"] = "Urban Art Core community celebrating street art and graffiti culture"
	case domain.PlatformMastodon:
		vars["PLATFORM_CONSTRAINTS"] = "500 characters, thoughtful and detailed"
		vars["TONE"] = "thoughtful, educational, community-building"
		vars["COMMUNITY_CONTEXT"] = "Urban Art Core community discussing street art history and cultural impact"
	}

	// Add date information
	vars["DATE_KEY"] = req.DateKey
	if len(req.Posts) > 0 {
		vars["ORIGINAL_DATE"] = req.Posts[0].Date().Format("January 2, 2006")
	}

	// Add any custom context from request
	for key, value := range req.Context {
		vars[strings.ToUpper(key)] = value
	}

	return vars
}

// ValidateTemplate checks if a prompt template is syntactically correct.
func (pm *PromptManager) ValidateTemplate(ctx context.Context, template string) error {

	if pm == nil {
		return PromptManagerError{

			Err: fmt.Errorf("prompt manager is nil"),
		}
	}

	// Check context cancellation
	select {
	case <-ctx.Done():
		return PromptManagerError{Err: ctx.Err()}
	default:
	}

	// Check non-empty
	template = strings.TrimSpace(template)
	if template == "" {
		return PromptManagerError{

			Err: fmt.Errorf("template cannot be empty"),
		}
	}

	// Check minimum length
	if len(template) < 50 {
		return PromptManagerError{

			Err: fmt.Errorf("template is too short (minimum 50 characters): got %d", len(template)),
		}
	}

	// Extract all template variables
	vars := templateVarPattern.FindAllString(template, -1)

	// Log found variables for debugging
	slog.Default().Debug("Template validation",
		slog.Int("length", len(template)),
		slog.Int("variable_count", len(vars)),
		slog.Any("variables", vars))

	return nil
}

// ListPrompts returns all available prompt templates by platform.
func (pm *PromptManager) ListPrompts(ctx context.Context) (map[string]string, error) {

	if pm == nil {
		return nil, PromptManagerError{

			Err: fmt.Errorf("prompt manager is nil"),
		}
	}

	// Check context cancellation
	select {
	case <-ctx.Done():
		return nil, PromptManagerError{Err: ctx.Err()}
	default:
	}

	pm.mu.RLock()
	defer pm.mu.RUnlock()

	// Create a copy to avoid external modification
	result := make(map[string]string, len(pm.templates))
	maps.Copy(result, pm.templates)

	return result, nil
}
