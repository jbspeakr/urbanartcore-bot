# OpenTofu Infrastructure - UAC Socializer

Production infrastructure as code for deploying UAC Socializer to Scaleway Serverless Jobs.

## Overview

Deploys the application as a daily scheduled Scaleway Serverless Job with:
- **Cron scheduling** (configurable time/timezone, default: midnight UTC)
- **Secrets management** via Scaleway Secret Manager
- **Environment variables** automatically populated from `.env`
- **Cost-effective**: ~€0.04/month for daily execution

## Architecture

```
.env (source of truth)
    ↓
justfile loads .env → exports TF_VAR_* env vars
    ↓
OpenTofu reads TF_VAR_* variables
    ↓
Creates:
├── scaleway_job_definition (daily cron job)
├── scaleway_secrets (3 credentials in Secret Manager)
└── scaleway_secret_versions (secret versions)
    ↓
Scaleway Serverless Job executes on schedule
├── Reads env vars from job definition
├── Retrieves secrets from Secret Manager
└── Runs container image
```

## Quick Start

See **CLAUDE.md** for the complete workflow.

Summary:
```bash
just tofu-init      # First time: initialize
just tofu-plan      # Review changes
just tofu-apply     # Deploy
```

Variables are automatically loaded from `.env` in the project root.

## Configuration

### Environment Variables (from .env)

All configuration comes from `.env` file (see CLAUDE.md for full reference):

**Scaleway:**
```
SCW_ACCESS_KEY
SCW_SECRET_KEY
SCW_DEFAULT_PROJECT_ID
SCW_REGION              (default: fr-par)
SCW_ZONE                (default: fr-par-1)
SCW_REGISTRY            (default: rg.fr-par.scw.cloud)
SCW_IMAGE_NAME          (default: brennenstuhl/uac-socializer)
```

**Application (mapped to job environment):**
```
UAC_MISTRAL_API_KEY
UAC_BLUESKY_PASSWORD
UAC_MASTODON_ACCESS_TOKEN
UAC_BLUESKY_HANDLE
UAC_MASTODON_SERVER
UAC_MISTRAL_MODEL       (optional, default: mistral-large-2512)
UAC_MISTRAL_MAX_TOKENS  (optional, default: 1024)
UAC_MISTRAL_TEMPERATURE (optional, default: 0.7)
UAC_AI_RETRY_COUNT      (optional, default: 3)
UAC_AI_TIMEOUT          (optional, default: 30s)
```

### Job Definition

Located in `main.tf`:

```hcl
resource "scaleway_job_definition" "uac_socializer" {
  name         = "uac-socializer"
  cpu_limit    = 1000                    # 1 vCPU (sufficient for AI)
  memory_limit = 512                     # 512 MiB RAM
  timeout      = "10m"                   # Job timeout
  image_uri    = "${registry}/${image}:latest"
  command      = "publish"               # Run 'publish' subcommand
  region       = var.region
  project_id   = var.SCW_DEFAULT_PROJECT_ID
}
```

### Secrets Management

Sensitive credentials stored in Scaleway Secret Manager:
- `UAC_MISTRAL_API_KEY`
- `UAC_BLUESKY_PASSWORD`
- `UAC_MASTODON_ACCESS_TOKEN`

Marked with `protected = true` to prevent accidental deletion.

## Files Reference

| File | Purpose |
|------|---------|
| `variables.tf` | Variable definitions (maps to .env keys) |
| `provider.tf` | Scaleway provider configuration |
| `main.tf` | Job definition resource |
| `secrets.tf` | Secret Manager resources |
| `locals.tf` | Computed values (image URI, env vars) |
| `outputs.tf` | Export job ID and console URL |
| `.terraform/` | OpenTofu state and provider (gitignored) |
| `.terraform.lock.hcl` | Provider version lock file |
| `.gitignore` | Prevents accidental commits of state files |

## Variable Mapping

OpenTofu variables are automatically mapped from `.env` keys:

| .env Variable | OpenTofu Variable |
|---|---|
| `SCW_DEFAULT_PROJECT_ID` | `var.SCW_DEFAULT_PROJECT_ID` |
| `SCW_REGISTRY` | `var.SCW_REGISTRY` |
| `SCW_IMAGE_NAME` | `var.SCW_IMAGE_NAME` |
| `UAC_MISTRAL_API_KEY` | `var.UAC_MISTRAL_API_KEY` |
| `UAC_BLUESKY_PASSWORD` | `var.UAC_BLUESKY_PASSWORD` |
| `UAC_MASTODON_ACCESS_TOKEN` | `var.UAC_MASTODON_ACCESS_TOKEN` |
| `UAC_BLUESKY_HANDLE` | `var.UAC_BLUESKY_HANDLE` |
| `UAC_MASTODON_SERVER` | `var.UAC_MASTODON_SERVER` |

(Plus optional AI configuration variables)

The justfile automatically exports `.env` variables as `TF_VAR_*` environment variables before running tofu commands.

## Testing Before Production

Deploy in dry-run mode first:

```bash
just push-image
just tofu-apply -var 'job_command=["publish", "--dry-run"]'
```

View logs:
- Scaleway Console: https://console.scaleway.com/serverless/jobs
- Click job name → "Execution history" tab
- Verify content generation without publishing

Switch to production when ready:
```bash
just tofu-apply
```

## Monitoring

View execution history and logs:
- https://console.scaleway.com/serverless/jobs
- Click job name
- Select "Execution history" tab
- View logs for current/past executions

Key metrics to monitor:
- **Next execution time** - Verify cron schedule
- **Execution status** - Success or failure
- **Duration** - Typical 30-90 seconds
- **Log output** - Content generation and publish results

## Troubleshooting

### "Image not found" error
**Cause:** Container image not pushed to registry.
```bash
just push-image
just tofu-apply
```

### "Unauthorized" errors
**Cause:** Invalid credentials.
1. Verify credentials in `.env`
2. Test locally: `go run ./main.go publish --dry-run`
3. Update `.env` with correct values
4. Run: `just tofu-apply`

### "server URL cannot be empty"
**Cause:** Mastodon server URL not set or malformed.
```bash
# Verify in .env:
UAC_MASTODON_SERVER="https://mastodon.social"  # Include https://
```

### Job doesn't execute on schedule
**Cause:** Cron configuration or timezone issue.
1. Verify schedule with https://crontab.guru
2. Wait for next scheduled time (don't expect immediate execution)
3. Manually trigger job in Scaleway Console to test

### Secrets appear in logs
**Cause:** Secrets not properly referenced.
- Application automatically redacts secrets in logs via `SanitizeSecrets()`
- No action needed—this is by design

### "top_p must be in (0, 1]"
**Cause:** Mistral API parameter validation.
- This is fixed in the application. Ensure using latest Docker image.
- Run: `just push-image && just tofu-apply`

## Cost Analysis

For default configuration (1 vCPU, 512 MiB, daily execution):

| Component | Cost/Month |
|-----------|-----------|
| Serverless Job (2 min/day) | ~€0.02 |
| Secret Manager | Free (3 secrets) |
| Container Registry | ~€0.02 |
| **Total** | **~€0.04** |

**Yearly: ~€0.50** (extremely cost-effective)

Other frequencies:
- Twice daily: ~€0.48/year
- Hourly: ~€5.76/year

## Maintenance

### Updating Credentials

```bash
# Edit .env with new credentials
vim .env

# Apply changes
just tofu-apply
```

Secrets in Secret Manager are automatically updated.

### Updating Container Image

When deploying new code:
```bash
git commit -am "Your changes"
just deploy  # Builds, pushes, and deploys
```

OpenTofu detects the new image and updates the job on next execution.

### Changing Configuration

```bash
# Edit .env
vim .env

# Preview changes
just tofu-plan

# Apply
just tofu-apply
```

Job is updated in-place with no downtime.

### Destroying Infrastructure

⚠️ **Warning:** This will delete the job and secrets.

```bash
just tofu-destroy
```

Secrets are marked `protected = true` and must be explicitly managed to prevent accidental deletion.

## State Management

OpenTofu state file (`terraform.tfstate`) tracks infrastructure:
- **Never** delete manually
- **Never** commit to version control (in `.gitignore`)
- Back it up regularly if managing multiple environments
- For local development only, it's safe to lose

State location: `tofu/.terraform/`

## Security Considerations

✅ **Implemented:**
- Secrets stored in Scaleway Secret Manager (not in state files)
- Sensitive variables marked in `variables.tf`
- Application automatically redacts secrets from logs
- Job runs with minimal permissions
- No hardcoded credentials in code

✅ **Best Practices:**
- `.env` never committed (in `.gitignore`)
- OpenTofu state not committed
- All credentials via environment variables
- Regular credential rotation recommended

## References

- OpenTofu: https://opentofu.org/docs/
- Scaleway Provider: https://registry.terraform.io/providers/scaleway/scaleway/latest/docs
- Cron Scheduling: https://crontab.guru
- Application Config: See CLAUDE.md
- Container Image: rg.fr-par.scw.cloud/brennenstuhl/uac-socializer:latest

## Support

For issues:
1. Check Scaleway Console for job logs
2. Verify OpenTofu plan before applying: `just tofu-plan`
3. Test locally: `go run ./main.go publish --dry-run`
4. Verify credentials and environment variables in `.env`
5. See AGENTS.md for application troubleshooting

---

See **AGENTS.md** for application-level configuration and deployment workflow.
