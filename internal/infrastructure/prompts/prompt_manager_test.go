package prompts

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jbspeakr/urbanartcore-bot/internal/domain"
	"github.com/jbspeakr/urbanartcore-bot/internal/ports"
)

// createTestPromptDir creates a temporary directory with test prompt templates
func createTestPromptDir(t *testing.T) string {
	tempDir := t.TempDir()

	blueskyTemplate := `Test prompt for Bluesky
Date: {DATE_KEY}
Years: {ANNIVERSARY_YEARS}
URI: {POST_URI}
Constraints: {PLATFORM_CONSTRAINTS}
Tone: {TONE}
Context: {COMMUNITY_CONTEXT}`

	mastodonTemplate := `Test prompt for Mastodon
Date: {DATE_KEY}
Years: {ANNIVERSARY_YEARS}
URI: {POST_URI}
Original: {ORIGINAL_DATE}
Constraints: {PLATFORM_CONSTRAINTS}
Tone: {TONE}
Context: {COMMUNITY_CONTEXT}`

	err := os.WriteFile(filepath.Join(tempDir, "bluesky.txt"), []byte(blueskyTemplate), 0644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(tempDir, "mastodon.txt"), []byte(mastodonTemplate), 0644)
	require.NoError(t, err)

	return tempDir
}

// createTestPost creates a test post for generation requests
func createTestPost() *domain.Post {
	post, _ := domain.NewPost("https://www.urbanartcore.com/test-post/", "2009-09-14")
	return post
}

// TestNewPromptManager tests the constructor
func TestNewPromptManager(t *testing.T) {
	tests := []struct {
		name        string
		setup       func() string
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid directory with templates",
			setup: func() string {
				return createTestPromptDir(t)
			},
			expectError: false,
		},
		{
			name: "empty path",
			setup: func() string {
				return ""
			},
			expectError: true,
			errorMsg:    "cannot be empty",
		},
		{
			name: "non-existent directory",
			setup: func() string {
				return filepath.Join(t.TempDir(), "nonexistent")
			},
			expectError: true,
			errorMsg:    "does not exist",
		},
		{
			name: "path is a file not directory",
			setup: func() string {
				tempDir := t.TempDir()
				filePath := filepath.Join(tempDir, "file.txt")
				err := os.WriteFile(filePath, []byte("test"), 0644)
				require.NoError(t, err)
				return filePath
			},
			expectError: true,
			errorMsg:    "not a directory",
		},
		{
			name: "missing bluesky template",
			setup: func() string {
				tempDir := t.TempDir()
				// Only create mastodon template
				err := os.WriteFile(filepath.Join(tempDir, "mastodon.txt"), []byte("test prompt"), 0644)
				require.NoError(t, err)
				return tempDir
			},
			expectError: true,
			errorMsg:    "template file not found",
		},
		{
			name: "empty template file",
			setup: func() string {
				tempDir := t.TempDir()
				err := os.WriteFile(filepath.Join(tempDir, "bluesky.txt"), []byte(""), 0644)
				require.NoError(t, err)
				err = os.WriteFile(filepath.Join(tempDir, "mastodon.txt"), []byte("test"), 0644)
				require.NoError(t, err)
				return tempDir
			},
			expectError: true,
			errorMsg:    "empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			promptsDir := tt.setup()

			pm, err := NewPromptManager(promptsDir)

			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
				assert.Nil(t, pm)
			} else {
				require.NoError(t, err)
				require.NotNil(t, pm)
				assert.Equal(t, promptsDir, pm.promptsDir)
				assert.Len(t, pm.templates, 2) // bluesky + mastodon
			}
		})
	}
}

// TestGetPrompt tests retrieving prompt templates
func TestGetPrompt(t *testing.T) {
	promptsDir := createTestPromptDir(t)
	pm, err := NewPromptManager(promptsDir)
	require.NoError(t, err)

	tests := []struct {
		name        string
		platform    string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid bluesky platform",
			platform:    "bluesky",
			expectError: false,
		},
		{
			name:        "valid mastodon platform",
			platform:    "mastodon",
			expectError: false,
		},
		{
			name:        "case insensitive platform",
			platform:    "BLUESKY",
			expectError: false,
		},
		{
			name:        "platform with whitespace",
			platform:    "  mastodon  ",
			expectError: false,
		},
		{
			name:        "invalid platform",
			platform:    "twitter",
			expectError: true,
			errorMsg:    "invalid platform",
		},
		{
			name:        "empty platform",
			platform:    "",
			expectError: true,
			errorMsg:    "invalid platform",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			template, err := pm.GetPrompt(ctx, tt.platform)

			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
				assert.Empty(t, template)
			} else {
				require.NoError(t, err)
				assert.NotEmpty(t, template)
				assert.Contains(t, template, "{DATE_KEY}")
				assert.Contains(t, template, "{POST_URI}")
			}
		})
	}
}

// TestGetPrompt_ContextCancellation tests context cancellation
func TestGetPrompt_ContextCancellation(t *testing.T) {
	promptsDir := createTestPromptDir(t)
	pm, err := NewPromptManager(promptsDir)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	template, err := pm.GetPrompt(ctx, "bluesky")
	require.Error(t, err)
	assert.Empty(t, template)
	assert.Contains(t, err.Error(), "context")
}

// TestGetPrompt_NilManager tests nil manager handling
func TestGetPrompt_NilManager(t *testing.T) {
	var pm *PromptManager
	ctx := context.Background()

	template, err := pm.GetPrompt(ctx, "bluesky")
	require.Error(t, err)
	assert.Empty(t, template)
	assert.Contains(t, err.Error(), "nil")
}

// TestRenderPrompt tests template rendering with variable substitution
func TestRenderPrompt(t *testing.T) {
	promptsDir := createTestPromptDir(t)
	pm, err := NewPromptManager(promptsDir)
	require.NoError(t, err)

	tests := []struct {
		name            string
		platform        string
		setupRequest    func() *ports.GenerationRequest
		expectError     bool
		errorMsg        string
		validateContent func(*testing.T, string)
	}{
		{
			name:     "successful render for bluesky",
			platform: "bluesky",
			setupRequest: func() *ports.GenerationRequest {
				post := createTestPost()
				req, _ := ports.NewGenerationRequest([]*domain.Post{post}, "bluesky", "09-14")
				return req
			},
			expectError: false,
			validateContent: func(t *testing.T, rendered string) {
				assert.Contains(t, rendered, "09-14")                                   // DATE_KEY
				assert.Contains(t, rendered, "https://www.urbanartcore.com/test-post/") // POST_URI
				assert.NotContains(t, rendered, "{DATE_KEY}")                           // Variable should be replaced
				assert.NotContains(t, rendered, "{POST_URI}")                           // Variable should be replaced
				assert.Contains(t, rendered, "300 bytes")                               // Platform constraints
			},
		},
		{
			name:     "successful render for mastodon",
			platform: "mastodon",
			setupRequest: func() *ports.GenerationRequest {
				post := createTestPost()
				req, _ := ports.NewGenerationRequest([]*domain.Post{post}, "mastodon", "09-14")
				return req
			},
			expectError: false,
			validateContent: func(t *testing.T, rendered string) {
				assert.Contains(t, rendered, "09-14")
				assert.Contains(t, rendered, "https://www.urbanartcore.com/test-post/")
				assert.NotContains(t, rendered, "{DATE_KEY}")
				assert.Contains(t, rendered, "500 characters") // Platform constraints
			},
		},
		{
			name:     "nil request",
			platform: "bluesky",
			setupRequest: func() *ports.GenerationRequest {
				return nil
			},
			expectError: true,
			errorMsg:    "cannot be nil",
		},
		{
			name:     "invalid platform",
			platform: "twitter",
			setupRequest: func() *ports.GenerationRequest {
				post := createTestPost()
				req, _ := ports.NewGenerationRequest([]*domain.Post{post}, "bluesky", "09-14")
				return req
			},
			expectError: true,
			errorMsg:    "invalid platform",
		},
		{
			name:     "request with custom context",
			platform: "bluesky",
			setupRequest: func() *ports.GenerationRequest {
				post := createTestPost()
				req, _ := ports.NewGenerationRequest([]*domain.Post{post}, "bluesky", "09-14")
				req.WithContext("CUSTOM_VAR", "custom value")
				return req
			},
			expectError: false,
			validateContent: func(t *testing.T, rendered string) {
				assert.Contains(t, rendered, "09-14")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			req := tt.setupRequest()

			rendered, err := pm.RenderPrompt(ctx, tt.platform, req)

			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
				assert.Empty(t, rendered)
			} else {
				require.NoError(t, err)
				assert.NotEmpty(t, rendered)
				if tt.validateContent != nil {
					tt.validateContent(t, rendered)
				}
			}
		})
	}
}

// TestRenderPrompt_NilManager tests nil manager handling
func TestRenderPrompt_NilManager(t *testing.T) {
	var pm *PromptManager
	ctx := context.Background()
	post := createTestPost()
	req, _ := ports.NewGenerationRequest([]*domain.Post{post}, "bluesky", "09-14")

	rendered, err := pm.RenderPrompt(ctx, "bluesky", req)
	require.Error(t, err)
	assert.Empty(t, rendered)
	assert.Contains(t, err.Error(), "nil")
}

// TestBuildVariableMap tests variable map creation
func TestBuildVariableMap(t *testing.T) {
	promptsDir := createTestPromptDir(t)
	pm, err := NewPromptManager(promptsDir)
	require.NoError(t, err)

	post := createTestPost()
	req, _ := ports.NewGenerationRequest([]*domain.Post{post}, "bluesky", "09-14")
	req.WithContext("CUSTOM_KEY", "custom_value")

	vars := pm.buildVariableMap(req)

	// Verify core variables
	assert.Contains(t, vars, "DATE_KEY")
	assert.Equal(t, "09-14", vars["DATE_KEY"])

	assert.Contains(t, vars, "POST_URI")
	assert.Equal(t, "https://www.urbanartcore.com/test-post/", vars["POST_URI"])

	assert.Contains(t, vars, "ANNIVERSARY_YEARS")
	// Anniversary years should be calculated from 2009 to current year

	// Verify platform-specific variables
	assert.Contains(t, vars, "PLATFORM_CONSTRAINTS")
	assert.Contains(t, vars, "TONE")
	assert.Contains(t, vars, "COMMUNITY_CONTEXT")

	// Verify custom context
	assert.Contains(t, vars, "CUSTOM_KEY")
	assert.Equal(t, "custom_value", vars["CUSTOM_KEY"])
}

// TestValidateTemplate tests template validation
func TestValidateTemplate(t *testing.T) {
	promptsDir := createTestPromptDir(t)
	pm, err := NewPromptManager(promptsDir)
	require.NoError(t, err)

	tests := []struct {
		name        string
		template    string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid template",
			template:    "This is a valid template with sufficient length for validation and some variables {DATE_KEY}",
			expectError: false,
		},
		{
			name:        "empty template",
			template:    "",
			expectError: true,
			errorMsg:    "cannot be empty",
		},
		{
			name:        "whitespace only template",
			template:    "   \n\t  ",
			expectError: true,
			errorMsg:    "cannot be empty",
		},
		{
			name:        "too short template",
			template:    "Short",
			expectError: true,
			errorMsg:    "too short",
		},
		{
			name:        "template with variables",
			template:    "Template with {VAR1} and {VAR2} and sufficient length for validation requirements",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			err := pm.ValidateTemplate(ctx, tt.template)

			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestValidateTemplate_ContextCancellation tests context cancellation
func TestValidateTemplate_ContextCancellation(t *testing.T) {
	promptsDir := createTestPromptDir(t)
	pm, err := NewPromptManager(promptsDir)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = pm.ValidateTemplate(ctx, "valid template with sufficient length")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "context")
}

// TestValidateTemplate_NilManager tests nil manager handling
func TestValidateTemplate_NilManager(t *testing.T) {
	var pm *PromptManager
	ctx := context.Background()

	err := pm.ValidateTemplate(ctx, "test template")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nil")
}

// TestListPrompts tests listing all available prompts
func TestListPrompts(t *testing.T) {
	promptsDir := createTestPromptDir(t)
	pm, err := NewPromptManager(promptsDir)
	require.NoError(t, err)

	ctx := context.Background()
	prompts, err := pm.ListPrompts(ctx)

	require.NoError(t, err)
	assert.Len(t, prompts, 2)
	assert.Contains(t, prompts, "bluesky")
	assert.Contains(t, prompts, "mastodon")

	// Verify prompts are not empty
	assert.NotEmpty(t, prompts["bluesky"])
	assert.NotEmpty(t, prompts["mastodon"])

	// Verify returned map is a copy (modifications don't affect internal state)
	prompts["bluesky"] = "modified"
	originalPrompt, _ := pm.GetPrompt(ctx, "bluesky")
	assert.NotEqual(t, "modified", originalPrompt)
}

// TestListPrompts_ContextCancellation tests context cancellation
func TestListPrompts_ContextCancellation(t *testing.T) {
	promptsDir := createTestPromptDir(t)
	pm, err := NewPromptManager(promptsDir)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	prompts, err := pm.ListPrompts(ctx)
	require.Error(t, err)
	assert.Nil(t, prompts)
	assert.Contains(t, err.Error(), "context")
}

// TestListPrompts_NilManager tests nil manager handling
func TestListPrompts_NilManager(t *testing.T) {
	var pm *PromptManager
	ctx := context.Background()

	prompts, err := pm.ListPrompts(ctx)
	require.Error(t, err)
	assert.Nil(t, prompts)
	assert.Contains(t, err.Error(), "nil")
}

// TestConcurrentAccess tests thread-safe concurrent access to templates
func TestConcurrentAccess(t *testing.T) {
	promptsDir := createTestPromptDir(t)
	pm, err := NewPromptManager(promptsDir)
	require.NoError(t, err)

	ctx := context.Background()
	post := createTestPost()

	// Run concurrent operations
	done := make(chan bool)
	for i := range 10 {
		go func(n int) {
			defer func() { done <- true }()

			platform := "bluesky"
			if n%2 == 0 {
				platform = "mastodon"
			}

			// Test GetPrompt
			_, err := pm.GetPrompt(ctx, platform)
			assert.NoError(t, err)

			// Test RenderPrompt
			req, _ := ports.NewGenerationRequest([]*domain.Post{post}, platform, "09-14")
			_, err = pm.RenderPrompt(ctx, platform, req)
			assert.NoError(t, err)

			// Test ListPrompts
			_, err = pm.ListPrompts(ctx)
			assert.NoError(t, err)
		}(i)
	}

	// Wait for all goroutines
	for range 10 {
		<-done
	}
}

// TestPromptManagerError tests error type behavior
func TestPromptManagerError(t *testing.T) {
	tests := []struct {
		name         string
		err          PromptManagerError
		expectMsg    string
		expectUnwrap error
	}{
		{
			name: "error with platform",
			err: PromptManagerError{
				Op:       "TestOp",
				Platform: "bluesky",
				Err:      assert.AnError,
			},
			expectMsg:    "prompt_manager[bluesky].TestOp:",
			expectUnwrap: assert.AnError,
		},
		{
			name: "error without platform",
			err: PromptManagerError{
				Op:  "TestOp",
				Err: assert.AnError,
			},
			expectMsg:    "prompt_manager.TestOp:",
			expectUnwrap: assert.AnError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errMsg := tt.err.Error()
			assert.Contains(t, errMsg, tt.expectMsg)

			unwrapped := tt.err.Unwrap()
			assert.Equal(t, tt.expectUnwrap, unwrapped)
		})
	}
}

// TestInterfaceCompliance verifies PromptManager implements the interface
func TestInterfaceCompliance(t *testing.T) {
	var _ ports.PromptManager = (*PromptManager)(nil)
}

// TestRealTemplateFiles tests loading the actual template files
func TestRealTemplateFiles(t *testing.T) {
	// This test uses the real template files from the project
	// Skip if running in CI without the actual files
	promptsDir := "../prompts"
	if _, err := os.Stat(promptsDir); os.IsNotExist(err) {
		t.Skip("Real template files not found, skipping test")
	}

	pm, err := NewPromptManager(promptsDir)
	require.NoError(t, err)

	ctx := context.Background()

	// Test Bluesky template
	blueskyTemplate, err := pm.GetPrompt(ctx, "bluesky")
	require.NoError(t, err)
	assert.Contains(t, blueskyTemplate, "Bluesky")
	assert.Contains(t, blueskyTemplate, "{POST_URI}")
	assert.Contains(t, blueskyTemplate, "{DATE_KEY}")
	assert.Contains(t, blueskyTemplate, "300 character") // Bluesky char limit

	// Test Mastodon template
	mastodonTemplate, err := pm.GetPrompt(ctx, "mastodon")
	require.NoError(t, err)
	assert.Contains(t, mastodonTemplate, "Mastodon")
	assert.Contains(t, mastodonTemplate, "{POST_URI}")
	assert.Contains(t, mastodonTemplate, "{DATE_KEY}")
	assert.Contains(t, mastodonTemplate, "500 character") // Mastodon char limit

	// Test rendering with real templates
	post := createTestPost()
	req, _ := ports.NewGenerationRequest([]*domain.Post{post}, "bluesky", "09-14")

	rendered, err := pm.RenderPrompt(ctx, "bluesky", req)
	require.NoError(t, err)
	assert.NotEmpty(t, rendered)
	assert.NotContains(t, rendered, "{POST_URI}") // Should be substituted
	assert.Contains(t, rendered, "https://www.urbanartcore.com/test-post/")
}

// TestTemplateVariablePattern tests the regex pattern for template variables
func TestTemplateVariablePattern(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "single variable with underscore",
			input:    "Text with {DATE_KEY} inside",
			expected: []string{"{DATE_KEY}"},
		},
		{
			name:     "multiple variables",
			input:    "Text with {POST_URI} and {DATE_KEY} and {PLATFORM}",
			expected: []string{"{POST_URI}", "{DATE_KEY}", "{PLATFORM}"},
		},
		{
			name:     "no variables",
			input:    "Text without any variables",
			expected: nil,
		},
		{
			name:     "lowercase not matched",
			input:    "Text with {lowercase} should not match",
			expected: nil,
		},
		{
			name:     "underscore in variable",
			input:    "Text with {VAR_NAME} and {ANOTHER_VAR}",
			expected: []string{"{VAR_NAME}", "{ANOTHER_VAR}"},
		},
		{
			name:     "actual template variables",
			input:    "{ANNIVERSARY_YEARS} since {ORIGINAL_DATE}",
			expected: []string{"{ANNIVERSARY_YEARS}", "{ORIGINAL_DATE}"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := templateVarPattern.FindAllString(tt.input, -1)
			assert.Equal(t, tt.expected, matches)
		})
	}
}

// TestVariableSubstitution_EdgeCases tests edge cases in variable substitution
func TestVariableSubstitution_EdgeCases(t *testing.T) {
	promptsDir := createTestPromptDir(t)
	pm, err := NewPromptManager(promptsDir)
	require.NoError(t, err)

	t.Run("empty posts array", func(t *testing.T) {
		req := &ports.GenerationRequest{
			Posts:    []*domain.Post{},
			Platform: "bluesky",
			DateKey:  "09-14",
			Context:  make(map[string]string),
		}

		vars := pm.buildVariableMap(req)
		// Should not panic, but won't have post-specific variables
		assert.Contains(t, vars, "DATE_KEY")
		assert.Contains(t, vars, "PLATFORM_CONSTRAINTS")
	})

	t.Run("platform-specific variables", func(t *testing.T) {
		post := createTestPost()

		// Test Bluesky
		reqBsky, _ := ports.NewGenerationRequest([]*domain.Post{post}, "bluesky", "09-14")
		varsBsky := pm.buildVariableMap(reqBsky)
		assert.Contains(t, strings.ToLower(varsBsky["PLATFORM_CONSTRAINTS"]), "byte")
		assert.Contains(t, strings.ToLower(varsBsky["TONE"]), "casual")

		// Test Mastodon
		reqMast, _ := ports.NewGenerationRequest([]*domain.Post{post}, "mastodon", "09-14")
		varsMast := pm.buildVariableMap(reqMast)
		assert.Contains(t, strings.ToLower(varsMast["PLATFORM_CONSTRAINTS"]), "character")
		assert.Contains(t, strings.ToLower(varsMast["TONE"]), "thoughtful")
	})
}

// TestSlugWords tests turning a post URL into the words of its slug
func TestSlugWords(t *testing.T) {
	tests := map[string]string{
		"https://www.urbanartcore.com/graffiti-meets-thalys-in-paris/": "graffiti meets thalys in paris",
		"https://www.urbanartcore.com/banksy_revealed":                 "banksy revealed",
		"https://www.urbanartcore.com/2011/09/roa-in-london.html":      "roa in london",
		"not a url": "not a url",
	}
	for uri, want := range tests {
		assert.Equal(t, want, slugWords(uri), uri)
	}
}

// TestRealTemplatesAreHonest checks the shipped prompts no longer claim the
// model can read the post, and that every variable they use is filled.
func TestRealTemplatesAreHonest(t *testing.T) {
	pm, err := NewPromptManager("../prompts")
	require.NoError(t, err)

	post, err := domain.NewPost("https://www.urbanartcore.com/graffiti-meets-thalys-in-paris/", "2009-09-14")
	require.NoError(t, err)

	for _, platform := range []string{domain.PlatformBluesky, domain.PlatformMastodon} {
		req, err := ports.NewGenerationRequest([]*domain.Post{post}, platform, "09-14")
		require.NoError(t, err)

		rendered, err := pm.RenderPrompt(context.Background(), platform, req)
		require.NoError(t, err)

		assert.NotContains(t, strings.ToLower(rendered), "read the blog post", platform)
		assert.Contains(t, rendered, "You cannot open the URL", platform)
		assert.Contains(t, rendered, `"graffiti meets thalys in paris"`, platform)
		assert.Contains(t, rendered, "September 2009", platform)
		assert.Empty(t, templateVarPattern.FindAllString(rendered, -1), platform)
	}
}
