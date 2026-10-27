# Ledger: one JSON document per year in Object Storage, recording every
# published post so a rerun posts nothing and posts do not repeat within a
# year. The job reaches the bucket with an API key of its own.

resource "scaleway_object_bucket" "ledger" {
  name       = var.ledger_bucket
  region     = var.region
  project_id = var.SCW_DEFAULT_PROJECT_ID

  # Earlier versions of each yearly document stay recoverable
  versioning {
    enabled = true
  }

  tags = local.common_tags

  lifecycle {
    prevent_destroy = true
  }
}

resource "scaleway_iam_application" "ledger" {
  name        = "${var.app_name}-ledger"
  description = "UAC Socializer job: reads and writes the ledger bucket"
}

resource "scaleway_iam_policy" "ledger" {
  name           = "${var.app_name}-ledger"
  description    = "Object Storage access for the UAC Socializer ledger"
  application_id = scaleway_iam_application.ledger.id

  rule {
    project_ids          = [var.SCW_DEFAULT_PROJECT_ID]
    permission_set_names = ["ObjectStorageBucketsRead", "ObjectStorageObjectsRead", "ObjectStorageObjectsWrite"]
  }
}

resource "scaleway_iam_api_key" "ledger" {
  application_id     = scaleway_iam_application.ledger.id
  description        = "UAC Socializer ledger"
  default_project_id = var.SCW_DEFAULT_PROJECT_ID
}

resource "scaleway_secret" "ledger_secret_key" {
  name        = "${var.app_name}-ledger-secret-key"
  description = "Secret key of the API key the UAC Socializer uses for its ledger bucket"
  region      = var.region
  protected   = true

  tags = [for k, v in local.common_tags : "${k}:${v}"]

  lifecycle {
    prevent_destroy = true
  }
}

resource "scaleway_secret_version" "ledger_secret_key" {
  secret_id = scaleway_secret.ledger_secret_key.id
  data      = scaleway_iam_api_key.ledger.secret_key
}
