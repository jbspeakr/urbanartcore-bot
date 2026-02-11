locals {
  # Resource naming
  job_name = "${var.app_name}-job"

  # Full image URI with tag
  image_uri = "${var.SCW_REGISTRY}/${var.SCW_IMAGE_NAME}:${var.image_version}"

  # Non-sensitive environment variables
  env_vars = {
    UAC_DATA_PATH              = var.data_path
    UAC_PROMPTS_PATH           = var.prompts_path
    UAC_BLUESKY_HANDLE         = var.UAC_BLUESKY_HANDLE
    UAC_MASTODON_SERVER        = var.UAC_MASTODON_SERVER
    UAC_MISTRAL_MODEL          = var.mistral_model
    UAC_MISTRAL_MAX_TOKENS     = tostring(var.mistral_max_tokens)
    UAC_MISTRAL_TEMPERATURE    = tostring(var.mistral_temperature)
    UAC_AI_RETRY_COUNT         = tostring(var.ai_retry_count)
    UAC_AI_TIMEOUT             = var.ai_timeout
    UAC_AI_MAX_RETRY_COUNT     = tostring(var.ai_retry_count)
    UAC_AI_MIN_RETRY_DELAY     = "1s"
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
