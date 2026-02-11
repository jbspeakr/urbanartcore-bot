# Scaleway Secret Manager resources for sensitive credentials
# All secrets marked as protected to prevent accidental deletion in production

resource "scaleway_secret" "mistral_api_key" {
  name        = "${var.app_name}-mistral-api-key"
  description = "Mistral AI API key for UAC Socializer"
  region      = var.region
  protected   = true

  tags = local.common_tags

  lifecycle {
    prevent_destroy = true
  }
}

resource "scaleway_secret_version" "mistral_api_key" {
  secret_id = scaleway_secret.mistral_api_key.id
  data      = var.UAC_MISTRAL_API_KEY
}

resource "scaleway_secret" "bluesky_password" {
  name        = "${var.app_name}-bluesky-password"
  description = "Bluesky app password for UAC Socializer"
  region      = var.region
  protected   = true

  tags = local.common_tags

  lifecycle {
    prevent_destroy = true
  }
}

resource "scaleway_secret_version" "bluesky_password" {
  secret_id = scaleway_secret.bluesky_password.id
  data      = var.UAC_BLUESKY_PASSWORD
}

resource "scaleway_secret" "mastodon_access_token" {
  name        = "${var.app_name}-mastodon-access-token"
  description = "Mastodon access token for UAC Socializer"
  region      = var.region
  protected   = true

  tags = local.common_tags

  lifecycle {
    prevent_destroy = true
  }
}

resource "scaleway_secret_version" "mastodon_access_token" {
  secret_id = scaleway_secret.mastodon_access_token.id
  data      = var.UAC_MASTODON_ACCESS_TOKEN
}
