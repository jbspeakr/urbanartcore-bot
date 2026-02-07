# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

**UAC Socializer** is a 12-factor Go CLI application that publishes historic anniversary blog posts to Bluesky and Mastodon. It reads posts from `data/posts_by_date.json` (organized by MM-DD date keys), selects posts for the current or specified date, generates platform-optimized content via Mistral AI, and publishes to social platforms with rich formatting (ATProto facets/embeds for Bluesky, hashtags/media for Mastodon).

**Key Characteristics:**
- Cobra/Viper CLI with 12-factor configuration
- Hexagonal architecture (ports & adapters pattern)
- Mistral AI for content enhancement
- All secrets via environment variables
- Containerized for serverless deployment
- Structured logging with slog

## Architecture

This project follows **Hexagonal Architecture** (Ports and Adapters) principles to ensure clean separation of concerns, testability, and platform extensibility.

### Core Architecture Layers

```
┌─────────────────────┐
│   CLI (Cobra)       │ ← Commands, flag parsing, output formatting
├─────────────────────┤
│   Application       │ ← Use cases, orchestration (PublishService)
├─────────────────────┤
│   Ports (Interfaces)│ ← PostRepository, SocialPublisher, ContentGenerator
├─────────────────────┤
│   Domain            │ ← Post, PublishResult (pure business logic)
├─────────────────────┤
│   Adapters          │ ← JSON file repo, Bluesky, Mastodon, Mistral implementations
├─────────────────────┤
│   Infrastructure    │ ← Config, logging, DI container
└─────────────────────┘
```

### Package Structure

- **`cmd/`** - CLI entry points
  - `root.go` - Cobra root command with Viper config initialization
  - `publish.go` - Publish subcommand (--dry-run, --date MM-DD, --platform)

- **`internal/domain/`** - Core business entities (zero external dependencies)
  - `post.go` - Post entity with validation
  - `result.go` - PublishResult value object for tracking publish outcomes

- **`internal/ports/`** - Interface contracts (hexagon boundary)
  - `repository.go` - PostRepository interface (data access)
  - `publisher.go` - SocialPublisher interface (platform publishing)
  - `generator.go` - ContentGenerator interface (AI content generation)

- **`internal/adapters/`** - External service implementations
  - `repository/json_file.go` - JSONFileRepository adapter ✅
  - `publisher/bluesky.go` - BlueskyAdapter (ATProto with facets/embeds) - Phase 4
  - `publisher/mastodon.go` - MastodonAdapter (Mastodon API) - Phase 4
  - `generator/mistral.go` - MistralAdapter (Mistral AI integration) - Phase 5

- **`internal/app/`** - Application layer
  - `publish.go` - PublishService use case orchestration
  - `config.go` - Configuration with Viper and env var binding

- **`internal/infrastructure/`** - Cross-cutting concerns
  - `container.go` - Dependency injection container setup
  - `logger.go` - Structured logging initialization (slog)

- **`data/`** - Data files
  - `posts_by_date.json` - MM-DD keyed post collection (source data)

### AI Content Generation Flow

1. **Post Selection**: Choose anniversary posts for the current date
2. **Content Generation**: Use Mistral AI with platform-specific prompts
3. **Content Validation**: Ensure generated content meets platform constraints
4. **Multi-Platform Publishing**: Publish tailored content to each platform

### Benefits of This Architecture

- **Testability**: Domain logic and AI integration isolated from external dependencies
- **Platform Switching**: Easy to add new social media platforms
- **AI Provider Flexibility**: Can switch from Mistral to other LLM providers
- **Content Customization**: Platform-specific prompts for optimal engagement
- **Maintainability**: Clear separation between business rules and technical details

## Build and Test Commands

### Building
```bash
# Build the main binary
go build ./cmd/uac-socializer

# Build with specific output name
go build -o bin/uac-socializer ./cmd/uac-socializer

# Cross-compile for Docker
CGO_ENABLED=0 GOOS=linux go build -o bin/uac-socializer ./cmd/uac-socializer
```

### Testing
```bash
# Run all tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Run specific test
go test -run TestMistralContentGeneration ./internal/adapters/mistral/

# Run tests for specific package
go test ./internal/domain/

# Test AI integration with actual API (requires API key)
go test -tags=integration ./internal/adapters/mistral/
```

### Code Quality
```bash
# Static analysis
go vet ./...

# Format code
go fmt ./...

# Lint (requires golangci-lint installed)
golangci-lint run

# Tidy dependencies
go mod tidy
```

### Local Development
```bash
# Set required environment variables
export UAC_MISTRAL_API_KEY="your-mistral-api-key"
export UAC_BLUESKY_HANDLE="your-handle.bsky.social"
export UAC_BLUESKY_PASSWORD="your-app-password"
export UAC_MASTODON_ACCESS_TOKEN="your-access-token"
export UAC_MASTODON_SERVER="https://mastodon.social"

# Run with dry-run mode (generates content but doesn't publish)
go run ./cmd/uac-socializer publish --dry-run

# Publish today's posts to all platforms with AI-generated content
go run ./cmd/uac-socializer publish

# Publish specific date to Bluesky only
go run ./cmd/uac-socializer publish --date 09-14 --platform bluesky

# Test content generation only
go run ./cmd/uac-socializer generate --date 09-14 --preview
```

## Data Model

### Posts JSON Structure
The `data/posts_by_date.json` file uses MM-DD keys to organize anniversary posts:

```json
{
  "09-14": [
    {
      "uri": "https://www.urbanartcore.com/graffiti-meets-thalys-in-paris/",
      "date": "2009-09-14"
    },
    {
      "uri": "https://www.urbanartcore.com/biggest-marker-worldwide/",
      "date": "2010-09-14"
    }
  ]
}
```

### Domain Entities

**Post**: Core entity representing a blog post
- `URI` - Full URL to the blog post (parsed by Mistral AI for content extraction)
- `Date` - Original publication date (YYYY-MM-DD format)
- Validation for URL format and date constraints
- Minimal data model - content extraction delegated to Mistral AI

**GeneratedContent**: AI-generated content with platform-specific optimization
- `Text` - Generated post text
- `Platform` - Target platform (bluesky, mastodon)
- `Hashtags` - Platform-appropriate hashtags
- `Metadata` - Character count, engagement predictions
- `PromptUsed` - Original prompt for debugging/optimization

**PublishResult**: Aggregates publishing outcomes across platforms
- Success/failure status per platform
- Generated content performance metrics
- Error messages and retry information
- AI generation costs and token usage

## AI Content Generation

### Mistral AI Integration

The application uses Mistral AI to generate engaging, platform-specific content with the model specified in configuration.

**Content Generation Flow**:
1. **Context Building**: Combine post metadata with anniversary context
2. **Prompt Selection**: Choose platform-specific prompt template
3. **AI Generation**: Call Mistral API with structured prompts
4. **Content Validation**: Ensure character limits and format compliance
5. **Quality Control**: Basic coherence and appropriateness checks

### Platform-Specific Prompts

**Bluesky Prompts** (`internal/infrastructure/prompts/bluesky.txt`):
- Conversational, community-focused tone
- Emphasizes visual culture and artistic heritage
- Includes appropriate hashtags (#StreetArt, #Graffiti, #UrbanArt)
- Optimized for 300-character limit
- Encourages engagement and discussion

**Mastodon Prompts** (`internal/infrastructure/prompts/mastodon.txt`):
- More detailed, thoughtful commentary
- Focus on cultural and historical significance
- Community-building language
- Utilizes 500-character limit effectively
- Includes content warnings where appropriate

### Prompt Engineering Strategy

**Template Structure**:
```
CONTEXT: [Anniversary date and historical significance]
POST_URI: [Full URL - Mistral extracts content from this]
PLATFORM: [Target platform with specific constraints]
TONE: [Community-focused, enthusiastic, educational]
REQUIREMENTS: [Character limits, hashtag expectations, engagement goals]
OUTPUT_FORMAT: [Structured response with text, hashtags, metadata]
```

**Dynamic Variables**:
- `{ANNIVERSARY_YEARS}` - Years since original publication
- `{POST_URI}` - Blog post URL (Mistral handles content extraction)
- `{PLATFORM_CONSTRAINTS}` - Character limits and formatting rules
- `{COMMUNITY_CONTEXT}` - UAC community-specific language
- `{ENGAGEMENT_HOOKS}` - Platform-appropriate call-to-actions

## Development Workflow

### Adding a New Social Platform

1. **Define the adapter interface** in `internal/ports/social_publisher.go`
2. **Create prompt template** in `internal/infrastructure/prompts/{platform}.txt`
3. **Implement the adapter** in `internal/adapters/{platform}/`
4. **Add platform-specific content validation** in domain layer
5. **Add configuration** for platform credentials in `internal/infrastructure/config.go`
6. **Wire up dependency** in `internal/app/container.go`
7. **Add platform option** to CLI flags in `cmd/publish.go`
8. **Write unit tests** with mocked AI responses and HTTP calls

### Adding New AI Features

1. **Extend AI contract** in `internal/ports/ai_content_generator.go`
2. **Update domain models** with new content types
3. **Modify Mistral adapter** with new prompt templates
4. **Add configuration options** for model selection and parameters
5. **Implement content validation** for new formats
6. **Write comprehensive tests** including AI integration tests

### Configuration Management

All secrets and configuration via environment variables:

**Core Configuration**:
- `UAC_MISTRAL_API_KEY` - Mistral AI API key
- `UAC_MISTRAL_MODEL` - Model selection (default: mistral-large)
- `UAC_MISTRAL_MAX_TOKENS` - Token limit per request
- `UAC_MISTRAL_TEMPERATURE` - Creativity level (0.0-1.0)

**Platform Configuration**:
- `UAC_BLUESKY_HANDLE` - Bluesky account handle
- `UAC_BLUESKY_PASSWORD` - Bluesky app password
- `UAC_MASTODON_ACCESS_TOKEN` - Mastodon access token
- `UAC_MASTODON_SERVER` - Mastodon instance URL

**Operational Configuration**:
- `UAC_DATA_PATH` - Custom path to posts JSON file
- `UAC_PROMPTS_PATH` - Custom path to prompt templates
- `UAC_AI_RETRY_COUNT` - AI generation retry attempts
- `UAC_AI_TIMEOUT` - AI request timeout duration

### Dependency Injection

Uses a container pattern in `internal/app/container.go`:
- **AI Service**: Mistral client with configuration
- **Repository**: JSON file reader with caching
- **Publishers**: Platform-specific publishers with auth
- **Prompt Manager**: Template loader with variable substitution
- **Config**: Environment-based configuration loader

## CLI Command Structure

Built on **Cobra** and **Viper** for robust CLI experience:

### Root Command
- `uac-socializer` - Base command with global flags
- `--config` - Custom configuration file path
- `--verbose` - Enable debug logging
- Uses Viper for configuration precedence (flags > env > config file)

### Publish Command
```bash
uac-socializer publish [flags]

Flags:
  --dry-run              Preview generated content without publishing (recommended for testing)
  --date MM-DD           Publish specific anniversary date (default: today)
  --platform string      Target platform: bluesky|mastodon|all (default: all)
```

**Example Usage:**
```bash
# Preview today's posts without publishing
./uac-socializer publish --dry-run

# Publish to Bluesky only for a specific date
./uac-socializer publish --date 01-15 --platform bluesky

# Publish to all platforms (actual publish)
./uac-socializer publish
```

### Command Flow
1. **Parse flags** and load configuration
2. **Initialize AI client** with Mistral API credentials
3. **Load prompt templates** for target platforms
4. **Initialize infrastructure** container with all dependencies
5. **Execute use case** with AI-powered content generation
6. **Handle errors** with structured logging and user feedback
7. **Return appropriate exit codes** for CI/CD integration

## Error Handling

### Structured Error Types
- **DomainError**: Business rule violations (invalid dates, malformed URLs)
- **ApplicationError**: Use case failures (no posts found, validation failures)
- **InfrastructureError**: External service failures (API errors, network issues)
- **AIGenerationError**: Mistral API failures, content validation issues
- **PlatformError**: Social media platform-specific errors

### AI-Specific Error Handling
- **Rate Limit Handling**: Exponential backoff for API limits
- **Content Validation**: Reject inappropriate or malformed AI responses
- **Retry Logic**: Exponential backoff with configurable retry count
- **Token Limit Management**: Handle context window limitations gracefully
- **Cost Controls**: Track and limit AI API usage

### Logging Strategy
Uses Go's structured logging (`slog`) with AI-specific fields:
- **Debug**: AI prompts, responses, and generation parameters
- **Info**: Content generation success, platform publishing results
- **Warn**: Retry attempts, content modifications, rate limit warnings
- **Error**: AI failures, platform errors, critical system issues

## Structured Logging (slog) Requirements

### Logging Architecture

The application uses Go's standard `log/slog` package with a global logger. Logger initialization is hardcoded to JSON format and info log level, with automatic secret sanitization.

**Initialization** (in `cmd/publish.go`):
```go
opts := &slog.HandlerOptions{
    Level:       slog.LevelInfo,
    ReplaceAttr: infrastructure.SanitizeSecrets,
}
handler := slog.NewJSONHandler(os.Stdout, opts)
slog.SetDefault(slog.New(handler))
```

**Usage**:
- Get the global logger anywhere: `logger := slog.Default()`
- Works in production and tests - no special setup needed for tests
- Secrets automatically redacted via SanitizeSecrets function

### Mandatory Implementation Rule
**No `fmt.Print*()`, `fmt.Println()`, `log.*()`, or `println()` statements in production code.**

All logging must use `slog.Default()` throughout the application. This enables:
- Consistent JSON log format (hardcoded)
- Machine-parseable output for log aggregation
- Structured field extraction for monitoring
- Automatic secret sanitization (API keys, tokens, passwords)
- Single global logger instance (no DI complexity)

### Logging by Architectural Layer

**Domain Layer** (`internal/domain/`)
- Zero logging - pure business logic with no external dependencies
- All state communicated through return values and errors

**Ports Layer** (`internal/ports/`)
- Zero logging - interface contracts remain clean
- Logging handled in adapter implementations

**Application Layer** (`internal/app/`)
- Info: Successful use case completion, important state transitions, major operation steps
- Warn: Recoverable issues, retries
- Error: Use case failures, validation errors, dependency failures

**Adapter Layer** (`internal/adapters/`)
- Info: Successful external operations (file I/O, API calls, publishes)
- Warn: Rate limiting, retries
- Error: External service failures, connection errors, validation failures

**Command Layer** (`cmd/`)
- Info: User-facing feedback (command results, summaries, status, operation outcomes)
- Warn: Partial failures or degraded results
- Error: Critical failures with actionable messages

**Infrastructure Layer** (`internal/infrastructure/`)
- Info: Startup completion, configuration applied, container initialization
- Warn: Degraded modes
- Error: Initialization failures, connection errors

### Using slog.Default()

```go
import "log/slog"

func someFunction() {
    logger := slog.Default()

    logger.Info("Operation completed",
        slog.String("operation", "MyFunction"),
        slog.Int("items_processed", 42))

    if err != nil {
        logger.Error("Operation failed",
            slog.String("operation", "MyFunction"),
            slog.String("error", err.Error()))
        return err
    }
}
```

**Logger is at Info level** - Debug logs are filtered out. Use Info for user-visible feedback and operational events.

**Output Format**: All logs written as JSON to stdout, safe for container log aggregation (Docker, Kubernetes, etc.)

### Structured Field Convention

**Command and Application Layer** (user-facing and orchestration):
- Always include `"operation"` (string) - function/method name for traceability
- Include context fields: `platform`, `date`, `status`, `uri`, `mode`
- Include metrics: `character_count`, `token_count`, `estimated_cost_usd`, `duration`
- For error-level logs: include `error` field with error message

**Adapter and Repository Layer** (external service integration):
- Context-specific fields (e.g., `file_path`, `total_dates`, `platform`)
- Include relevant error details for debugging external failures
- Metrics specific to the adapter: `file_size`, `response_time`

**General Pattern**:
- Use descriptive field names that clarify what the value represents
- Include enough context to debug issues without looking at code
- Avoid generic field names - `platform` is better than `type`
- For errors: use `"error"` field with the error message

**Log Levels Used**:
- **Info**: Normal operations, user-facing feedback, successful completions
- **Warn**: Recoverable issues, fallbacks, expected failures
- **Error**: Critical failures that stop operations
- **Debug**: Filtered out (logger at Info level only)

### Secret Sanitization

- API keys, access tokens, and passwords are automatically redacted by `SanitizeSecrets()` function
- Applied to all logs via the `ReplaceAttr` hook in logger initialization (in `cmd/publish.go`)
- Sensitive keys detected: `password`, `token`, `key`, `secret`, `credential`, `api_key`, `access_token`, etc.
- All slog output is safe for log aggregation services (Datadog, CloudWatch, ELK, etc.)

Example automatic redaction:
```
Input:  slog.String("uac_mistral_api_key", "sk-proj-abcdef123456")
Output: slog.String("uac_mistral_api_key", "sk-***456")
```

No special action needed - sensitive fields are redacted automatically for every log.

### Error Handling and Logging

**Returning Errors from Functions**:
- Always wrap errors with context using ApplicationError or fmt.Errorf
- Include the operation name and relevant fields in the error message
- Return early from the function - let the caller decide whether to log
- Pattern: `return nil, ApplicationError{Op: op, Err: err}`

**Logging Errors**:
- Log at Error level for critical operational failures (command layer)
- Include structured error context: error message, operation, relevant fields
- Pattern: `logger.Error("operation failed", slog.String("error", err.Error()))`
- Don't log every error in every layer - apply selective logging based on layer

**Error Propagation**:
- Domain layer: Errors are never logged, only returned
- Adapter layer: Errors are wrapped with context, selectively logged for critical issues
- Application layer: Errors are returned with ApplicationError wrapper, not logged here
- Command layer: Errors from services are caught and logged before displaying to user

### 12-Factor Compliance
- **All logs go to stdout** in JSON format
- **No file writing or local log storage**
- **No log level configuration** - hardcoded to Info level
- **No log format configuration** - hardcoded to JSON
- Container/deployment infrastructure handles log aggregation (Docker, Kubernetes, cloud logging)
- Secrets automatically sanitized before logging

## Key Files Reference

### Currently Implemented - Core Files
- `main.go` - Application entry point
- `cmd/root.go` - Cobra root command with Viper configuration
- `cmd/publish.go` - Complete publish command implementation
- `internal/app/publish.go` - PublishService with full orchestration (6-step workflow)
- `internal/app/config.go` - Configuration management and validation
- `internal/infrastructure/container.go` - Dependency injection setup
- `internal/infrastructure/logger.go` - Structured logging with slog and secret sanitization

### Domain and Ports
- `internal/domain/post.go` - Post entity with validation
- `internal/domain/result.go` - GeneratedContent, PlatformResult, PublishResult domain models
- `internal/ports/repository.go` - PostRepository interface contract
- `internal/ports/publisher.go` - SocialPublisher interface contract
- `internal/ports/generator.go` - ContentGenerator interface contract

### Data Access Layer
- `internal/adapters/repository/json_file.go` - JSONFileRepository implementation
- `internal/adapters/repository/json_file_test.go` - Repository tests with 86.7% coverage
- `internal/adapters/repository/testdata/` - Test fixtures

### AI Integration Files
- `internal/adapters/generator/mistral.go` - Mistral API integration with full ContentGenerator implementation
- `internal/adapters/generator/mistral_test.go` - Comprehensive test suite for Mistral adapter
- `internal/infrastructure/prompts/prompt_manager.go` - Prompt template manager implementation
- `internal/infrastructure/prompts/bluesky.txt` - Bluesky-optimized prompt template
- `internal/infrastructure/prompts/mastodon.txt` - Mastodon-optimized prompt template

### Social Platform Adapters
- `internal/adapters/publisher/bluesky.go` - Bluesky/ATProto implementation
- `internal/adapters/publisher/bluesky_facets.go` - ATProto facet generation helpers
- `internal/adapters/publisher/mastodon.go` - Mastodon API implementation
- `internal/adapters/publisher/*_test.go` - Comprehensive test suites

### Configuration
- `.env.example` - Environment variable examples
- `internal/app/config.go` - Configuration loading from env vars

## Testing Strategy

### Unit Tests
- **Domain layer**: Content validation, entity behavior with AI-generated data
- **Application layer**: Use case orchestration, AI integration, error handling
- **Adapter layer**: Mistral API integration (with mocked HTTP responses)
- **Prompt Engineering**: Template rendering and variable substitution

### Integration Tests
- **End-to-end AI flow**: Full content generation and publishing pipeline
- **Platform integration**: Cross-platform content consistency
- **AI model testing**: Compare outputs across different Mistral models
- **Cost optimization**: Verify token usage and API efficiency

### AI Testing Strategy
- **Mock Mistral responses** for consistent unit testing
- **Golden file testing** for prompt template changes
- **Content quality metrics** for regression testing
- **Token usage tracking** for cost optimization
- **Platform compliance testing** for character limits and format rules

### Test Organization
- Use **testify** for assertions and test suites
- Mock AI responses with realistic content samples
- Test data includes various post types and anniversary scenarios
- Separate AI integration tests with build tags (`-tags=ai_integration`)
- Performance tests for AI generation speed and token efficiency

## Important Constraints

### Required Dependencies
- **Go 1.25.6+** - latest language features required
- `github.com/spf13/cobra` (v1.10.2+) - CLI framework
- `github.com/spf13/viper` (v1.21.0+) - Configuration management
- `github.com/ZaguanLabs/mistral-go` - Mistral AI integration (Phase 5)
- `github.com/disintegration/imaging` (v1.6.2) - Image resizing for link preview thumbnails (infrastructure utility)
- `golang.org/x/net` - HTML parsing for link preview extraction
- `github.com/stretchr/testify` - Test assertions and mocking

**Note:** Social platform adapters (Bluesky, Mastodon) use pure REST/HTTP over stdlib (`net/http`). No external platform-specific libraries required.

### Architectural Constraints
- **Hexagonal architecture**: All external integrations through ports/adapters
- **Zero business logic in adapters**: Domain layer has no external dependencies
- **Dependency injection**: All components wired through infrastructure container
- **Configuration via environment**: 12-factor app principles
- **Graceful degradation**: Continue publishing if one platform fails

### Security Requirements
- **All secrets via env vars**: Never hardcode API keys, tokens, or credentials
- **No secrets in logs**: Structured logging must exclude credentials
- **Validate all inputs**: Sanitize dates, platform names, user-provided data
- **Content validation**: Check AI responses for injection attacks
- **No temporary credential files**: Use in-memory auth only

### Mistral AI Integration Requirements
- **Always use Mistral AI**: Content generation is mandatory, not optional
- **Single model configuration**: Only the configured model is used (no fallback to alternative models)
- **URI parsing**: Mistral AI parses blog post URIs directly for content extraction (no pre-extracted title/summary)
- **Respect rate limits**: Implement exponential backoff for API rate limiting
- **Retry logic**: Exponential backoff on retryable errors, fail on non-retryable errors
- **Content validation**: Validate AI responses meet platform requirements

### Testing Requirements
- **testify for assertions**: Use `assert` and `require` packages
- **Mock all adapters**: Tests never hit real APIs
- **80%+ coverage target**: Full test coverage for domain and app layers
- **golangci-lint**: Must pass all linter checks
- **Table-driven tests**: Use pattern for comprehensive scenario coverage

### Deployment Constraints
- **Containerized**: Must build with `CGO_ENABLED=0 GOOS=linux`
- **Serverless-ready**: Stateless design, no local file persistence
- **CI/CD ready**: Passes tests on `.forgejo/workflows/publish.yml`
- **Date handling**: Always use MM-DD format (zero-padded)
- **Exit codes**: 0 for success, 1 for errors

## Current Project State

### Currently Implemented

**Infrastructure & Configuration:**
- `main.go` - Entry point delegating to cmd.Execute()
- `cmd/root.go` - Cobra root command with Viper configuration
- `cmd/publish.go` - Main publish command with full implementation
- `go.mod` - Dependencies defined (Cobra, Viper, Mistral, Bluesky, Mastodon, etc.)
- `.forgejo/workflows/publish.yml` - CI/CD pipeline with `go test ./...` and linting
- `data/posts_by_date.json` - Source data (extremely large, MM-DD keyed)
- `internal/infrastructure/{logger,container}.go` - Logging and DI setup
- `internal/app/config.go` - Configuration management with validation

**Phase 1: Foundation** ✅ COMPLETE
- ✅ Domain models (`internal/domain/{post,result}.go`)
  - Post: Minimal entity with URI + date (content extraction via Mistral AI)
  - GeneratedContent: AI-generated content with platform-specific optimization
  - PlatformResult: Platform-specific publishing outcome
  - PublishResult: Multi-platform publishing outcomes with cost tracking
- ✅ Port interfaces (`internal/ports/{repository,publisher,generator}.go`)
  - PostRepository: Data access contract
  - SocialPublisher: Platform-agnostic publishing interface
  - ContentGenerator: AI content generation contract
- ✅ Logging infrastructure (`internal/infrastructure/logger.go`)
  - Structured slog-based logging with secret sanitization
  - Hardcoded JSON format and Info level (12-factor compliant)
- ✅ Comprehensive unit tests with 89%+ coverage
- ✅ All compilation, linting, and test checks passing

**Phase 2: Data Access** ✅ COMPLETE
- ✅ JSONFileRepository adapter (`internal/adapters/repository/json_file.go`)
  - Full MM-DD date key parsing and validation
  - O(1) lookups with in-memory caching
  - Comprehensive error handling and test coverage (86.7%)
- ✅ Configuration management (`internal/app/config.go`)
  - Environment variable loading with Viper
  - Configuration validation with clear error messages
  - Sanitization for logging
- ✅ Dependency injection container (`internal/infrastructure/container.go`)
  - All dependencies properly wired with real adapters
  - Real implementations for all phases (repository, publishers, content generator)
  - Container validation and health checks with proper error handling
  - Test coverage handles adapter authentication requirements

**Phase 3: Core Use Case** ✅ COMPLETE
- ✅ PublishService (`internal/app/publish.go`)
  - Complete orchestration: repository → content generation → publishing
  - Multi-platform publishing workflow with proper error handling
  - Result aggregation with success/failure/skip tracking
  - Dry-run mode with mock content generation for testing
  - 6-step orchestration: validation → retrieval → platform resolution → generation → publishing → aggregation
  - Test coverage: 88.2%
- ✅ Publish command (`cmd/publish.go`)
  - Full Cobra command implementation with all flags
  - Flags: --dry-run, --date MM-DD, --platform (bluesky|mastodon|all)
  - Configuration loading and container initialization
  - Comprehensive output formatting for dry-run and live modes
  - Exit codes for CI/CD integration
- ✅ End-to-end workflow with dry-run mode
  - Full workflow operational and testable
  - Content generation preview with mock responses
  - Dry-run testing recommended before live publishing

**Phase 4: Social Platform Adapters** ✅ COMPLETE
- ✅ MastodonAdapter (`internal/adapters/publisher/mastodon.go`)
  - Implements SocialPublisher interface for Mastodon
  - Content validation for 500-character limit (rune-based)
  - Pure REST/HTTP calls (no external libraries)
  - OAuth Bearer token authentication
  - Error classification and retry logic
- ✅ BlueskyAdapter (`internal/adapters/publisher/bluesky.go`)
  - Implements SocialPublisher interface for Bluesky/ATProto
  - Content validation for 300-byte limit (UTF-8)
  - Pure REST/HTTP calls (no external libraries)
  - Session management with caching and expiry
  - ATProto facet generation for rich text (hashtags, links)
  - External embed support for link cards with previews
- ✅ Facet helpers (`internal/adapters/publisher/bluesky_facets.go`)
  - ByteRange validation and UTF-8 aware positioning
  - Hashtag and link facet generation
  - External embed building with metadata extraction
- ✅ Comprehensive unit tests (85%+ coverage for both adapters)
- ✅ All tests passing, zero dependencies on platform-specific libraries

**Phase 5: AI Integration** ✅ COMPLETE
- ✅ MistralAdapter (`internal/adapters/generator/mistral.go`)
  - Implements ContentGenerator interface
  - Integration with github.com/gage-technologies/mistral-go
  - Prompt template rendering with dynamic variables via PromptManager
  - Rate limit handling with exponential backoff retry logic
  - Comprehensive error classification (retryable vs non-retryable)
  - Single model support (configured via UAC_MISTRAL_MODEL)
- ✅ Prompt templates and validation
  - Platform-specific prompt files in `internal/infrastructure/prompts/`
  - PromptManager implementation with template variable substitution
  - Content validation against platform constraints (byte/rune counting)
  - AI error handling with exponential backoff retry mechanism
  - JSON response format enforcement for consistent parsing
- ✅ Full integration with container and PublishService
  - Real MistralAdapter wired into dependency injection container
  - Complete end-to-end workflow: posts → AI generation → platform publishing
  - Comprehensive test coverage with mocked responses

### To Be Implemented

**Phase 6: Testing & Quality**
- [ ] Integration tests with mock external services
- [ ] golangci-lint configuration
- [ ] Full test coverage (80%+ target for all layers)

**Phase 7: Containerization**
- [ ] Dockerfile for serverless deployment
- [ ] Cross-platform build automation
- [ ] Release artifacts

## Development Quick Start

```bash
# Build the binary
go build -o bin/uac-socializer .

# Run all tests
go test ./...

# Run tests with coverage report
go test -cover ./...

# Format and lint
go fmt ./... && go vet ./...

# Static analysis (if golangci-lint installed)
golangci-lint run

# Test dry-run mode (requires all credentials for adapter initialization)
export UAC_DATA_PATH="data/posts_by_date.json"
export UAC_MISTRAL_API_KEY="your-mistral-key"
export UAC_BLUESKY_HANDLE="your-handle.bsky.social"
export UAC_BLUESKY_PASSWORD="your-app-password"
export UAC_MASTODON_ACCESS_TOKEN="your-access-token"
export UAC_MASTODON_SERVER="https://mastodon.social"

# Dry-run mode (generates content via AI but doesn't publish)
./bin/uac-socializer publish --dry-run --date 09-14 --platform all

# Publish to all platforms with AI-generated content
./bin/uac-socializer publish --date 09-14 --platform all

# Publish today's posts to Bluesky only
./bin/uac-socializer publish --platform bluesky
```

---

## API Integration Strategy: Pure REST/HTTP

### Design Philosophy

All social media platform integrations use **pure REST/HTTP** over the Go standard library (`net/http`) with no external service libraries. This approach provides:

1. **Minimal Dependencies**: Reduces external library dependencies to just Cobra/Viper for CLI and testify for testing
2. **Maximum Clarity**: All HTTP calls are explicit and easily auditable
3. **Flexibility**: Can quickly adapt to API changes without waiting for library updates
4. **Consistency**: Both Bluesky and Mastodon adapters follow identical HTTP patterns
5. **Testability**: Simple to mock HTTP requests/responses in tests
6. **Smaller Binary**: No library bloat, faster builds

### Implementation Details

**MastodonAdapter** (`internal/adapters/publisher/mastodon.go`):
- Direct HTTP calls to Mastodon REST API
- `/api/v1/accounts/verify_credentials` for connection test
- `/api/v1/statuses` for posting (POST with Bearer token auth)
- Response parsing via `encoding/json` (stdlib)

**BlueskyAdapter** (`internal/adapters/publisher/bluesky.go`):
- Direct HTTP calls to ATProto/Bluesky REST API
- `/xrpc/com.atproto.server.createSession` for authentication
- `/xrpc/com.atproto.repo.createRecord` for posting
- Session caching with expiry management
- Facet generation for rich text (hashtags, links)
- External embed support for link cards

### Dependencies

**Required (Go stdlib only)**:
- `net/http` - HTTP client
- `encoding/json` - JSON marshaling/unmarshaling
- `context` - Request cancellation and timeouts
- `log/slog` - Structured logging (included in Go 1.21+)
- `golang.org/x/net/html` - HTML parsing for link preview extraction

**NOT used** (removed for simplicity):
- ~~`github.com/mattn/go-mastodon`~~ - Replaced with pure REST calls
- ~~`github.com/bluesky-social/indigo`~~ - Replaced with pure REST calls

### Error Handling

All adapter errors classified via standard HTTP status codes:
- `401`/`403` - Authentication failures (non-retryable)
- `429` - Rate limiting (retryable)
- `5xx` - Server errors (retryable)
- Network timeouts (retryable)
- Validation errors (non-retryable)

### Benefits Over Time

- **No dependency bloat**: Only core Cobra/Viper for CLI
- **Reduced attack surface**: Fewer external libraries to audit
- **Easier debugging**: See exact HTTP requests/responses
- **Faster iteration**: No waiting for library releases
- **Clear semantics**: HTTP status codes are self-documenting

### Architectural Exception: Image Processing Library

The `github.com/disintegration/imaging` library is used for image thumbnail resizing in the Bluesky adapter and is a documented exception to the "pure REST/HTTP over stdlib" philosophy:

**Rationale:**
1. **Infrastructure Utility vs. API Integration** - The imaging library performs general-purpose image processing (resizing, format conversion) for link preview thumbnails. This is infrastructure/utility work, not social media platform API integration.
2. **Domain Complexity** - High-quality image resampling (Lanczos interpolation) requires sophisticated algorithms. Implementing this with stdlib's lower-level `golang.org/x/image/draw` would add significant boilerplate without benefit.
3. **Clear Separation** - Image resizing occurs within the adapter's internal implementation for preparing rich link embeds. It does not replace REST/HTTP communication with Bluesky's ATProto API.
4. **Well-Maintained Library** - `disintegration/imaging` is a stable, single-purpose library with no external dependencies, making it a low-risk addition.

**Scope:** Image processing is limited to:
- Downloading link preview thumbnails from external URLs
- Resizing oversized images to ATProto blob size constraints (1000x1000px, 1MB)
- Converting to JPEG format for consistent encoding

This architecture provides a clean, testable foundation for social media automation with Mistral AI-powered content generation. All external integrations flow through well-defined ports and adapters, enabling easy platform switching and comprehensive testing.