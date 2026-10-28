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
UAC_SITE_BASE_URL="https://www.urbanartcore.com"   # optional; where index.json lives

# Ledger of published posts (optional locally; without it reruns post again)
UAC_LEDGER_BUCKET="uac-socializer-ledger"
UAC_LEDGER_ACCESS_KEY="SCW..."
UAC_LEDGER_SECRET_KEY="..."
UAC_LEDGER_ENDPOINT="https://s3.fr-par.scw.cloud"   # default
UAC_LEDGER_REGION="fr-par"                           # default

# Optional (defaults provided)
UAC_MISTRAL_MODEL="mistral-large-2512"
UAC_CRITIC_MODEL="ministral-8b-2512"   # checks every draft against its post
UAC_VISION_MODEL="mistral-large-2512"  # writes alt text for cover photos; empty uses the site's alt text
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
    site/                     # Catalogue and articles from the site's JSON
      snapshot/index.json     # Built-in catalogue, used when the site is down (`just snapshot`)
    publisher/                # Bluesky, Mastodon adapters
    generator/                # Mistral AI adapter
  app/
    publish.go                # PublishService use case
    config.go                 # Viper configuration
  infrastructure/
    container.go              # Dependency injection
    logger.go                 # slog structured logging
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
just plan [days]        # Print the coming days: program, post, score
just snapshot           # Refresh the built-in catalogue snapshot from the site
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
--program PROGRAM      # on-this-day|collection|artist-spotlight|back-then (default: on-this-day)
```

Example:
```bash
./uac-socializer publish --dry-run --date 09-14
./uac-socializer publish --platform bluesky
```

Planning and previews (catalogue and ledger only for `plan`; no posting for either):
```bash
uac-socializer plan --days 14 --from MM-DD   # Table of the coming days: program, post, score, why
uac-socializer draft --date 09-14            # Same as publish --dry-run
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
- Adapters: Bluesky, Mastodon, Mistral, site JSON
- Application: PublishService orchestration
- Infrastructure: DI container, logging, config
- CLI: Cobra commands

**Choosing the post:** the day's anniversary posts are scored (`internal/domain/plan.go`): 1 to start, +2 for a named artist, +1 for a place, +1 for a summary of 12 words or more, +1 for a round anniversary (5, 10, 15 years). A post featured within a year scores 0, and so does a link roundup (format `Link Tips`: links to other sites, whose artists and places get mixed up), and a day where nothing reaches 1 is a rest day. Ties go to the post never featured, then the one featured longest ago. `plan` uses the same scoring and never repeats a post within the plan.

**Thread programs:** `collection` posts one of the site's collections (found through `llms.txt`, each read from `collections/<slug>/index.json`) as a thread: title and description first, then each item in reading order with the collection's own note. `artist-spotlight` takes an artist with 5 or more posts, not spotlighted for half a year, and posts their first, a middle and their last post. `back-then` posts the slot's month, one post per year. The texts come from the site's titles and notes, so no model writes them. Bluesky replies carry link cards, Mastodon replies the link in the text; each reply answers the one before. The first post is recorded in the ledger under the collection, tag page or archive URL; a reply that fails ends the thread there.

**Ledger:** before choosing, the run reads the last year of records from the ledger bucket (`ledger/<year>.json`). Platforms that already have the slot's post are skipped (a rerun posts nothing; a partial run completes the missing platform with the same post), and posts featured within a year are passed over; a day with nothing else is a rest day. Each successful publish is appended straight away with a conditional PUT. An unreadable ledger stops the run; a failed append is logged, not fatal. Dry runs read but never write.

**Content pipeline (per platform):** read the post's JSON → writer drafts text plus a list of its claims → critic checks each claim against the post and scores the voice → a rejected draft is rewritten with the critic's findings, at most twice → if nothing passes, or the critic gives no verdict, the fallback template is posted instead. Dry runs log every review (`Draft reviewed`, with `unsupported` claims) and mark fallback content.

**Photos:** a post with a cover image carries it with alt text written by the vision model from the photo and the post's tags, ending with the photo credit. If the model fails, the site's alt text or the title stands in. Mastodon uploads the photo with the status; Bluesky keeps the link card, which shows the cover as its thumbnail (one embed per post). Dry runs log the image and its alt text.

**Design Principles:**
- 12-factor app (environment-based config)
- Stateless, containerized
- Pure REST/HTTP (no platform-specific libraries)
- Structured logging (slog) with automatic secret redaction
- All secrets via environment variables

## Key Implementation Files

- `cmd/publish.go` - Publish CLI command
- `internal/app/publish.go` - PublishService orchestration
- `internal/adapters/site/site.go` - Reads the site's index.json (catalogue) and each post's index.json (title, teaser, body, typed tags, cover, credit); falls back to the built-in snapshot
- `internal/domain/hashtags.go` - Hashtags from the post's typed tags (artist, place, technique, format, then #StreetArt/#Graffiti)
- `internal/adapters/publisher/bluesky.go` - Bluesky/ATProto integration
- `internal/adapters/publisher/mastodon.go` - Mastodon REST API
- `internal/adapters/generator/mistral.go` - Mistral AI integration
- `internal/infrastructure/prompts/voice.md` - System prompt: register, banned phrases, example posts
- `internal/infrastructure/prompts/` - Task templates per platform (`bluesky.txt`, `mastodon.txt`)
- `internal/adapters/generator/describer.go` - Alt text for cover photos from a Mistral model that accepts images
- `internal/adapters/publisher/mastodon_media.go` - Uploads images with their alt text to Mastodon before the status
- `internal/adapters/generator/critic.go` - Critic: a small Mistral model checks each claim of a draft against the post and scores the voice
- `internal/adapters/ledger/objectstorage.go` - Ledger in S3-compatible Object Storage, SigV4-signed requests, one JSON document per year
- `internal/domain/fallback.go` - Template post from title, artists, place and month, used when no draft passes review
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