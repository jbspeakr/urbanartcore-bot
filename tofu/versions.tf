terraform {
  required_version = ">= 1.6.0"

  required_providers {
    scaleway = {
      source  = "scaleway/scaleway"
      version = "~> 2.50"
    }
  }

  # Optional: Uncomment to use remote state storage
  # backend "s3" {
  #   bucket         = "uac-socializer-opentofu-state"
  #   key            = "prod/terraform.tfstate"
  #   region         = "fr-par"
  #   endpoint       = "https://s3.fr-par.scw.cloud"
  #   skip_credentials_validation = true
  #   skip_region_validation      = true
  # }
}
