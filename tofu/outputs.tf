output "job_id" {
  description = "Scaleway Job Definition ID"
  value       = scaleway_job_definition.uac_socializer.id
}

output "job_name" {
  description = "Deployed job name"
  value       = scaleway_job_definition.uac_socializer.name
}

output "image_uri" {
  description = "Full Docker image URI with tag"
  value       = local.image_uri
}

output "cron_schedule" {
  description = "Configured cron schedule (UTC)"
  value       = var.cron_schedule
}

output "cron_timezone" {
  description = "Timezone for cron schedule"
  value       = var.cron_timezone
}

output "secret_ids" {
  description = "Scaleway Secret Manager secret IDs"
  value = {
    mistral_api_key           = scaleway_secret.mistral_api_key.id
    bluesky_password          = scaleway_secret.bluesky_password.id
    mastodon_access_token     = scaleway_secret.mastodon_access_token.id
  }
}

output "deployment_info" {
  description = "Deployment configuration summary"
  value = {
    environment        = var.environment
    region             = var.region
    zone               = var.zone
    cpu_limit_mVCPU    = var.cpu_limit
    memory_limit_MiB   = var.memory_limit
    timeout            = var.timeout
    job_command        = join(" ", var.job_command)
    cron_schedule      = "${var.cron_schedule} ${var.cron_timezone}"
    mistral_model      = var.mistral_model
    mistral_max_tokens = var.mistral_max_tokens
  }
}

output "platform_config" {
  description = "Platform configuration"
  value = {
    bluesky_handle     = var.UAC_BLUESKY_HANDLE
    mastodon_server    = var.UAC_MASTODON_SERVER
  }
  sensitive = false
}

output "console_url" {
  description = "Direct link to job in Scaleway Console"
  value       = "https://console.scaleway.com/serverless/jobs/${var.SCW_DEFAULT_PROJECT_ID}/${scaleway_job_definition.uac_socializer.id}?region=${var.region}"
  sensitive   = true
}
