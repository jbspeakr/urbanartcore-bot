# CLAUDE.md - UAC Socializer v2

Go CLI application that publishes anniversary blog posts to Bluesky and Mastodon with AI-generated content via Mistral. Hexagonal architecture, 12-factor configuration, containerized for Scaleway Serverless Jobs.

## Quick Start (Local)

With a `.env` in the repository root (read automatically; real environment variables win), run:

```bash
go run ./main.go publish --dry-run                           # Preview
go run ./main.go publish --date 09-14 --platform bluesky    # Publish
```

## Build → Push → Deploy Workflow

### One-Command Deployment

```bash
# Commit changes first
git add . && git commit -m "Your changes"

# Then deploy everything (includes quality gates)
just deploy  # test → check → build → push → deploy
```

**Important:**
- Code must be committed before `just push-image` (container tagged with git commit hash)
- Tests must pass before deploying (included in `just check`)

### From GitHub Actions

`.github/workflows/image.yml` builds the image and pushes it to the Scaleway Container Registry (tagged with the commit SHA and `latest`) after CI passes on `main`, or by hand from the Actions tab. It uses the `production` environment: secret `SCW_SECRET_KEY`, optional variables `SCW_REGION` (default `fr-par`) and `SCW_REGISTRY_NAMESPACE` (default `brennenstuhl`). The job definition stays with OpenTofu and runs `:latest`.

### Step-by-Step

**1. Prepare .env**
```bash
cp .env.example .env  # Edit with your credentials
```

**2. Run tests**
```bash
just test
```

**3. Run quality checks**
```bash
just check  # fmt, vet, lint
```

**4. Commit changes**
```bash
git add . && git commit -m "Your changes"
```

**5. Build and push container**
```bash
just push-image
```
- Loads variables from `.env`
- Builds with Podman
- Tags as `rg.fr-par.scw.cloud/brennenstuhl/uac-socializer:latest`
- Image includes current commit hash and build timestamp
- Pushes to Scaleway Container Registry

**6. Deploy infrastructure**
```bash
just tofu-init     # First time only
just tofu-plan     # Review what will be created/changed
just tofu-apply    # Deploy to Scaleway Serverless Job
```

## Configuration (.env)

Single source of truth for all configuration. Never commit to version control.

```bash
# Scaleway infrastructure
SCW_ACCESS_KEY="..."
SCW_SECRET_KEY="..."
SCW_DEFAULT_PROJECT_ID="..."
SCW_REGISTRY="rg.fr-par.scw.cloud"
SCW_IMAGE_NAME="brennenstuhl/uac-socializer"

# Application secrets
UAC_MISTRAL_API_KEY="..."
UAC_BLUESKY_PASSWORD="..."
UAC_MASTODON_ACCESS_TOKEN="..."

# Platform configuration
UAC_BLUESKY_HANDLE="handle.bsky.social"
UAC_MASTODON_SERVER="https://mastodon.social"

# Optional (defaults provided)
UAC_MISTRAL_MODEL="mistral-large-2512"
UAC_MISTRAL_MAX_TOKENS="1024"
UAC_MISTRAL_TEMPERATURE="0.7"
UAC_AI_RETRY_COUNT="3"
UAC_AI_TIMEOUT="30s"
```

## Project Structure

```
cmd/                          # CLI commands
internal/
  domain/                     # Business logic
  ports/                      # Interface contracts
  adapters/
    repository/               # Data access (JSON file)
    publisher/                # Bluesky, Mastodon adapters
    generator/                # Mistral AI adapter
    reader/                   # Reads a post's web page for the prompt
  app/
    publish.go                # PublishService use case
    config.go                 # Viper configuration
  infrastructure/
    container.go              # Dependency injection
    logger.go                 # slog structured logging
data/posts_by_date.json       # Anniversary posts (MM-DD keys)
tofu/                         # OpenTofu infrastructure as code
justfile                      # Build automation
Dockerfile                    # Container definition
```

## Justfile Commands

```bash
just check              # Quality gates (fmt, vet, lint)
just test               # Run all tests
just push-image         # Build and push container
just build-local        # Build binary locally
just tofu-init          # Initialize OpenTofu
just tofu-plan          # Preview infrastructure changes
just tofu-apply         # Deploy infrastructure
just tofu-destroy       # Destroy infrastructure
just deploy             # Complete workflow (check → push → apply)
```

## Testing

**Must pass before deploying:**
```bash
just check                    # Quality checks (fmt, vet, lint, tests)
```

## Publishing Command

```bash
uac-socializer publish [flags]

--dry-run              # Preview without publishing
--date MM-DD           # Specific date (default: today)
--platform PLATFORM    # bluesky|mastodon|all (default: all)
```

Example:
```bash
./uac-socializer publish --dry-run --date 09-14
./uac-socializer publish --platform bluesky
```

## Production

Deployed to Scaleway Serverless Jobs:
- Daily scheduled execution via cron
- Credentials stored in Scaleway Secret Manager
- Infrastructure managed by OpenTofu
- Monthly cost: ~€0.04

Monitor: https://console.scaleway.com/serverless/jobs

## Architecture

**Hexagonal (Ports & Adapters):**
- Domain: Pure business logic (Post, PublishResult)
- Ports: Interface contracts
- Adapters: Bluesky, Mastodon, Mistral, JSON repository
- Application: PublishService orchestration
- Infrastructure: DI container, logging, config
- CLI: Cobra commands

**Design Principles:**
- 12-factor app (environment-based config)
- Stateless, containerized
- Pure REST/HTTP (no platform-specific libraries)
- Structured logging (slog) with automatic secret redaction
- All secrets via environment variables

## Key Implementation Files

- `cmd/publish.go` - Publish CLI command
- `internal/app/publish.go` - PublishService orchestration
- `internal/adapters/repository/json_file.go` - Data access
- `internal/adapters/publisher/bluesky.go` - Bluesky/ATProto integration
- `internal/adapters/publisher/mastodon.go` - Mastodon REST API
- `internal/adapters/generator/mistral.go` - Mistral AI integration
- `internal/adapters/reader/web.go` - Reads the post's title, teaser and opening text
- `internal/infrastructure/prompts/` - Prompt templates per platform
- `tofu/` - Infrastructure as code (see tofu/README.md)

## Important Constraints

- Go 1.25.6+ required
- All secrets via environment variables only
- Built with `CGO_ENABLED=0 GOOS=linux` for Linux deployment
- **Tests must pass** before deploying
- **Changes must be committed** before pushing (image tagged with git commit hash)
- OpenTofu state file is authoritative for infrastructure

---

See `tofu/README.md` for detailed infrastructure configuration.