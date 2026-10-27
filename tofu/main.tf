resource "scaleway_job_definition" "uac_socializer" {
  name                     = local.job_name
  cpu_limit                = var.cpu_limit
  memory_limit             = var.memory_limit
  local_storage_capacity   = var.local_storage_capacity
  timeout                  = var.timeout
  image_uri                = local.image_uri
  command                  = join(" ", var.job_command)
  region                   = var.region
  project_id               = var.SCW_DEFAULT_PROJECT_ID
  description              = "Daily UAC blog post publisher to Bluesky and Mastodon with AI-generated content"

  # Non-sensitive environment variables
  env = local.env_vars

  cron {
    schedule = var.cron_schedule
    timezone = var.cron_timezone
  }

  # Secrets reach the job as environment variables, always at their latest version
  dynamic "secret_reference" {
    for_each = local.secret_env
    content {
      environment    = secret_reference.key
      secret_id      = secret_reference.value
      secret_version = "latest"
    }
  }

  lifecycle {
    prevent_destroy = true
  }
}
