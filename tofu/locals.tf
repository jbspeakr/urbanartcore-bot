locals {
  # Resource naming
  job_name = "${var.app_name}-job"

  # Full image URI with tag
  image_uri = "${var.SCW_REGISTRY}/${var.SCW_IMAGE_NAME}:${var.image_version}"

  # Non-sensitive environment variables
  env_vars = {
    UAC_SITE_BASE_URL          = var.site_base_url
    UAC_PROMPTS_PATH           = var.prompts_path
    UAC_BLUESKY_HANDLE         = var.UAC_BLUESKY_HANDLE
    UAC_MASTODON_SERVER        = var.UAC_MASTODON_SERVER
    UAC_MISTRAL_MODEL          = var.mistral_model
    UAC_CRITIC_MODEL           = var.critic_model
    UAC_VISION_MODEL           = var.vision_model
    UAC_LEDGER_BUCKET          = scaleway_object_bucket.ledger.name
    UAC_LEDGER_ENDPOINT        = "https://s3.${var.region}.scw.cloud"
    UAC_LEDGER_REGION          = var.region
    UAC_LEDGER_ACCESS_KEY      = scaleway_iam_api_key.ledger.access_key
    UAC_MISTRAL_MAX_TOKENS     = tostring(var.mistral_max_tokens)
    UAC_MISTRAL_TEMPERATURE    = tostring(var.mistral_temperature)
    UAC_AI_RETRY_COUNT         = tostring(var.ai_retry_count)
    UAC_AI_TIMEOUT             = var.ai_timeout
    UAC_AI_MAX_RETRY_COUNT     = tostring(var.ai_retry_count)
    UAC_AI_MIN_RETRY_DELAY     = "1s"
  }

  # Secret Manager secrets exposed to the job, by environment variable
  secret_env = {
    UAC_MISTRAL_API_KEY       = trimprefix(scaleway_secret.mistral_api_key.id, "${var.region}/")
    UAC_BLUESKY_PASSWORD      = trimprefix(scaleway_secret.bluesky_password.id, "${var.region}/")
    UAC_MASTODON_ACCESS_TOKEN = trimprefix(scaleway_secret.mastodon_access_token.id, "${var.region}/")
    UAC_LEDGER_SECRET_KEY     = trimprefix(scaleway_secret.ledger_secret_key.id, "${var.region}/")
  }

  # Common tags for all resources
  common_tags = merge(
    var.common_tags,
    {
      created_by = "opentofu"
      region     = var.region
      zone       = var.zone
    }
  )
}
