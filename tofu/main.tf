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

  lifecycle {
    prevent_destroy = true
  }
}
