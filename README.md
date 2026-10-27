# UAC Socializer v2

Automated daily blog post publisher that generates platform-optimized content via Mistral AI and publishes to Bluesky and Mastodon.

## What Is This?

UAC Socializer reads anniversary blog posts from the urbanartcore.com JSON catalogue, uses Mistral AI to generate platform-specific content, and publishes to social media on a daily schedule. It's deployed as a serverless job on Scaleway and runs automatically every day at midnight UTC.

- ✅ Production-ready, fully tested
- 🤖 AI-powered content generation (Mistral)
- 🔐 Secure credential management
- 🏗️ Infrastructure as Code (OpenTofu)
- 💰 Cost-effective (~€0.04/month)

## Documentation

**Start here based on what you need:**

| I want to... | Read this |
|---|---|
| Deploy the application | [AGENTS.md](./AGENTS.md) |
| Understand infrastructure setup | [tofu/README.md](./tofu/README.md) |
| Run locally for development | [AGENTS.md - Quick Start](./AGENTS.md#quick-start-local) |
| See code architecture | [AGENTS.md - Architecture](./AGENTS.md#architecture) |
| Monitor production | [tofu/README.md - Monitoring](./tofu/README.md#monitoring) |

## Quick Start

```bash
# Setup
cp .env.example .env
# Edit .env with your credentials

# Test
go run ./main.go publish --dry-run

# Deploy
just deploy
```

See [AGENTS.md](./AGENTS.md) for full deployment guide.

## Commands

```bash
just test               # Run tests
just check              # Quality gates (test, fmt, vet, lint)
just push-image         # Build & push container
just deploy             # Complete workflow
just tofu-plan          # Preview infrastructure changes
just tofu-apply         # Deploy infrastructure
```

See [AGENTS.md - Justfile Commands](./AGENTS.md#justfile-commands) for complete reference.

## Project Status

✅ Fully implemented and production-ready

- Phase 1-7: Complete (domain, adapters, infrastructure, deployment)
- Test coverage: 85%+
- Deployment: Scaleway Serverless Jobs
- Monitoring: Scaleway Console

## License

See [LICENSE](./LICENSE) file.

---

**New to this project?** Start with [AGENTS.md](./AGENTS.md)
