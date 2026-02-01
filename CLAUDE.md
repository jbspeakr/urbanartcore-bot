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
│   Domain            │ ← Post, DateGroup, PublishResult (pure business logic)
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
  - `dategroup.go` - DateGroup value object for grouping posts by MM-DD
  - `result.go` - PublishResult value object for tracking publish outcomes

- **`internal/ports/`** - Interface contracts (hexagon boundary)
  - `repository.go` - PostRepository interface (data access)
  - `publisher.go` - SocialPublisher interface (platform publishing)
  - `generator.go` - ContentGenerator interface (AI content generation)

- **`internal/adapters/`** - External service implementations
  - `repository/json_file.go` - JSONFileRepository adapter
  - `publisher/bluesky.go` - BlueskyAdapter (ATProto with facets/embeds)
  - `publisher/mastodon.go` - MastodonAdapter (Mastodon API)
  - `generator/mistral.go` - MistralAdapter (Mistral AI integration)

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
- `URI` - Full URL to the blog post
- `Date` - Original publication date (YYYY-MM-DD format)
- `Title` - Extracted from URI or fetched from source
- `Summary` - Brief description for AI context
- Validation for URL format and date constraints

**DateGroup**: Value object for handling multiple posts on the same anniversary date
- Groups posts by MM-DD key
- Provides context for AI prompt generation
- Manages post selection when multiple posts exist

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

The application uses Mistral AI to generate engaging, platform-specific content:

**Model Selection**:
- Primary: `mistral-large` for high-quality content generation
- Fallback: `mistral-medium` for cost optimization
- Development: `mistral-small` for testing and iteration

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
POST_DATA: [URL, title, original date, brief description]
PLATFORM: [Target platform with specific constraints]
TONE: [Community-focused, enthusiastic, educational]
REQUIREMENTS: [Character limits, hashtag expectations, engagement goals]
OUTPUT_FORMAT: [Structured response with text, hashtags, metadata]
```

**Dynamic Variables**:
- `{ANNIVERSARY_YEARS}` - Years since original publication
- `{POST_TITLE}` - Extracted or generated title
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
- `UAC_LOG_LEVEL` - Logging level (debug, info, warn, error)
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
- **Fallback Strategies**: Use simpler models or cached content on failures
- **Token Limit Management**: Handle context window limitations gracefully
- **Cost Controls**: Track and limit AI API usage

### Logging Strategy
Uses Go's structured logging (`slog`) with AI-specific fields:
- **Debug**: AI prompts, responses, and generation parameters
- **Info**: Content generation success, platform publishing results
- **Warn**: AI fallbacks, content modifications, rate limit warnings
- **Error**: AI failures, platform errors, critical system issues

## Key Files Reference

### AI Integration Files
- `internal/adapters/mistral/client.go` - Mistral API integration
- `internal/adapters/mistral/prompts.go` - Prompt template management
- `internal/ports/ai_content_generator.go` - AI service contract
- `internal/infrastructure/prompts/` - Platform-specific prompt templates

### Critical Architecture Files
- `internal/app/container.go` - Dependency injection with AI services
- `internal/app/generate_content.go` - AI content generation use case
- `internal/domain/generated_content.go` - AI content domain model

### Command Implementation
- `cmd/root.go` - Cobra root command and configuration
- `cmd/publish.go` - Main publish command with AI integration
- `cmd/generate.go` - Content generation preview command
- `main.go` - Application entry point

### Configuration and Templates
- `internal/infrastructure/prompts/bluesky.txt` - Bluesky prompt template
- `internal/infrastructure/prompts/mastodon.txt` - Mastodon prompt template
- `.env.example` - Environment variable examples including AI config

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
- `github.com/ZaguanLabs/mistral-go` - Mistral AI integration
- `github.com/bluesky-social/indigo` - Bluesky/ATProto with facets and embeds
- `github.com/mattn/go-mastodon` - Mastodon API client
- `github.com/stretchr/testify` - Test assertions and mocking

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
- **Respect rate limits**: Implement exponential backoff for API rate limiting
- **Token management**: Monitor and report token usage
- **Fallback handling**: Handle API timeouts and service errors gracefully
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

### Implemented
- `main.go` - Entry point delegating to cmd.Execute()
- `cmd/root.go` - Cobra root command with Viper configuration
- `go.mod` - Dependencies defined (Cobra, Viper)
- `.forgejo/workflows/publish.yml` - CI/CD pipeline with `go test ./...`
- `data/posts_by_date.json` - Source data (extremely large, MM-DD keyed)

### To Be Implemented
This is an initial project setup. The architecture is defined but no core logic exists. Build in this sequence:

**Phase 1: Foundation**
- [ ] Create domain models (`internal/domain/{post,dategroup,result}.go`)
- [ ] Define port interfaces (`internal/ports/{repository,publisher,generator}.go`)
- [ ] Set up logging infrastructure (`internal/infrastructure/logger.go`)

**Phase 2: Data Access**
- [ ] JSONFileRepository adapter (`internal/adapters/repository/json_file.go`)
- [ ] Configuration management (`internal/app/config.go`)
- [ ] Dependency injection container (`internal/infrastructure/container.go`)

**Phase 3: Core Use Case**
- [ ] PublishService (`internal/app/publish.go`)
- [ ] Publish command (`cmd/publish.go`)
- [ ] Basic end-to-end flow with dry-run mode

**Phase 4: Social Platform Adapters**
- [ ] MastodonAdapter (`internal/adapters/publisher/mastodon.go`)
- [ ] BlueskyAdapter with ATProto (`internal/adapters/publisher/bluesky.go`)
- [ ] Platform-specific error handling and testing

**Phase 5: AI Integration**
- [ ] MistralAdapter (`internal/adapters/generator/mistral.go`)
- [ ] Prompt templates and content validation
- [ ] AI error handling and fallbacks

**Phase 6: Testing & Quality**
- [ ] Unit tests for all layers (80%+ coverage target)
- [ ] golangci-lint configuration
- [ ] Integration tests with real data

**Phase 7: Containerization**
- [ ] Dockerfile for serverless deployment
- [ ] Cross-platform build automation
- [ ] Release artifacts

## Development Quick Start

```bash
# Build current state
go build ./cmd/uac-socializer

# Run tests
go test ./...

# Format and lint
go fmt ./... && go vet ./...

# Local testing with required env vars
export UAC_MISTRAL_API_KEY="your-key"
export UAC_BLUESKY_HANDLE="your-handle"
export UAC_BLUESKY_PASSWORD="your-password"
export UAC_MASTODON_ACCESS_TOKEN="your-token"
export UAC_MASTODON_SERVER="https://mastodon.social"

# When ready, run with dry-run
./uac-socializer publish --dry-run --platform all
```

---

This architecture provides a clean, testable foundation for social media automation with Mistral AI-powered content generation. All external integrations flow through well-defined ports and adapters, enabling easy platform switching and comprehensive testing.