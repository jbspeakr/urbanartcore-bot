# Scaleway Provider Authentication
variable "SCW_DEFAULT_PROJECT_ID" {
  description = "Scaleway Project ID"
  type        = string
  sensitive   = true
  validation {
    condition     = length(var.SCW_DEFAULT_PROJECT_ID) > 0
    error_message = "SCW_DEFAULT_PROJECT_ID cannot be empty"
  }
}

variable "region" {
  description = "Scaleway region (e.g., fr-par, nl-ams, pl-waw)"
  type        = string
  default     = "fr-par"
  validation {
    condition     = contains(["fr-par", "nl-ams", "pl-waw", "us-east"], var.region)
    error_message = "region must be a valid Scaleway region"
  }
}

variable "zone" {
  description = "Scaleway availability zone (e.g., fr-par-1, fr-par-2)"
  type        = string
  default     = "fr-par-1"
  validation {
    condition     = can(regex("^[a-z]{2}-[a-z]{3}-[0-9]$", var.zone))
    error_message = "zone must be a valid Scaleway zone format"
  }
}

# Application Configuration
variable "app_name" {
  description = "Application name used in resource naming"
  type        = string
  default     = "uac-socializer"
  validation {
    condition     = can(regex("^[a-z0-9-]+$", var.app_name))
    error_message = "app_name must contain only lowercase letters, digits, and hyphens"
  }
}

variable "SCW_REGISTRY" {
  description = "Scaleway Container Registry URL (registry host and namespace)"
  type        = string
  default     = "rg.fr-par.scw.cloud/brennenstuhl"
  validation {
    condition     = can(regex("^[a-z0-9.-]+\\.scw\\.cloud/[a-z0-9-]+$", var.SCW_REGISTRY))
    error_message = "SCW_REGISTRY must be a valid Scaleway registry URI"
  }
}

variable "SCW_IMAGE_NAME" {
  description = "Docker image name (e.g., uac-socializer)"
  type        = string
  default     = "uac-socializer"
  validation {
    condition     = can(regex("^[a-z0-9-]+$", var.SCW_IMAGE_NAME))
    error_message = "SCW_IMAGE_NAME must contain only lowercase letters, digits, and hyphens"
  }
}

variable "image_version" {
  description = "Docker image version tag (e.g., latest, v1.2.3)"
  type        = string
  default     = "latest"
  validation {
    condition     = can(regex("^[a-z0-9.-]+$", var.image_version))
    error_message = "image_version must contain only lowercase letters, digits, dots, and hyphens"
  }
}

# Job Resource Configuration
variable "cpu_limit" {
  description = "CPU limit in mVCPU (70-4000, default 1000 = 1 vCPU)"
  type        = number
  default     = 1000
  validation {
    condition     = var.cpu_limit >= 70 && var.cpu_limit <= 4000
    error_message = "cpu_limit must be between 70 and 4000 mVCPU"
  }
}

variable "memory_limit" {
  description = "Memory limit in MiB (128-8192, default 512)"
  type        = number
  default     = 512
  validation {
    condition     = var.memory_limit >= 128 && var.memory_limit <= 8192
    error_message = "memory_limit must be between 128 and 8192 MiB"
  }
}

variable "local_storage_capacity" {
  description = "Local storage capacity in MiB (default 1000 = 1 GB)"
  type        = number
  default     = 1000
  validation {
    condition     = var.local_storage_capacity > 0 && var.local_storage_capacity <= 20000
    error_message = "local_storage_capacity must be greater than 0 and at most 20000 MiB"
  }
}

variable "timeout" {
  description = "Job timeout in Go duration format (e.g., 10m, 300s)"
  type        = string
  default     = "10m"
  validation {
    condition     = can(regex("^[0-9]+[smh]$", var.timeout))
    error_message = "timeout must be in Go duration format (e.g., 10m, 300s, 1h)"
  }
}

variable "job_command" {
  description = "Job command as list (e.g., [\"/uac-socializer\", \"publish\"])"
  type        = list(string)
  default     = ["/uac-socializer", "publish"]
  validation {
    condition     = length(var.job_command) > 0
    error_message = "job_command must contain at least one element"
  }
}

# Cron Configuration
variable "cron_schedule" {
  description = "Cron schedule for job execution"
  type        = string
  default     = "55 11 * * *"
  validation {
    condition     = can(regex("^(\\*|([0-9]|1[0-9]|2[0-9]|3[0-9]|4[0-9]|5[0-9])|\\*\\/([0-9]|1[0-9]|2[0-9]|3[0-9]|4[0-9]|5[0-9])) (\\*|([0-9]|1[0-9]|2[0-3])|\\*\\/([0-9]|1[0-9]|2[0-3])) (\\*|([1-9]|1[0-9]|2[0-9]|3[0-1])|\\*\\/([1-9]|1[0-9]|2[0-9]|3[0-1])) (\\*|([1-9]|1[0-2])|\\*\\/([1-9]|1[0-2])) (\\*|([0-6])|\\*\\/([0-6]))$", var.cron_schedule))
    error_message = "cron_schedule must be a valid 5-field cron expression"
  }
}

variable "cron_timezone" {
  description = "Timezone for cron schedule"
  type        = string
  default     = "Europe/Berlin"
  validation {
    condition     = contains(["UTC", "Europe/London", "Europe/Paris", "Europe/Berlin", "US/Eastern", "US/Pacific", "Asia/Tokyo"], var.cron_timezone)
    error_message = "cron_timezone must be a valid IANA timezone"
  }
}

# Sensitive Credentials (Scaleway Secret Manager)
variable "UAC_MISTRAL_API_KEY" {
  description = "Mistral AI API key (stored in Secret Manager)"
  type        = string
  sensitive   = true
  validation {
    condition     = length(var.UAC_MISTRAL_API_KEY) > 0
    error_message = "UAC_MISTRAL_API_KEY cannot be empty"
  }
}

variable "UAC_BLUESKY_PASSWORD" {
  description = "Bluesky app password (stored in Secret Manager)"
  type        = string
  sensitive   = true
  validation {
    condition     = length(var.UAC_BLUESKY_PASSWORD) > 0
    error_message = "UAC_BLUESKY_PASSWORD cannot be empty"
  }
}

variable "UAC_MASTODON_ACCESS_TOKEN" {
  description = "Mastodon access token (stored in Secret Manager)"
  type        = string
  sensitive   = true
  validation {
    condition     = length(var.UAC_MASTODON_ACCESS_TOKEN) > 0
    error_message = "UAC_MASTODON_ACCESS_TOKEN cannot be empty"
  }
}

# Application Configuration Variables (Non-sensitive)
variable "UAC_BLUESKY_HANDLE" {
  description = "Bluesky account handle (e.g., user.bsky.social or custom.domain)"
  type        = string
  validation {
    condition     = can(regex("^[a-z0-9.-]+$", var.UAC_BLUESKY_HANDLE))
    error_message = "UAC_BLUESKY_HANDLE must be a valid Bluesky handle (domain format)"
  }
}

variable "UAC_MASTODON_SERVER" {
  description = "Mastodon server URL (e.g., https://mastodon.social)"
  type        = string
  validation {
    condition     = can(regex("^https://[a-z0-9.-]+", var.UAC_MASTODON_SERVER))
    error_message = "UAC_MASTODON_SERVER must be a valid HTTPS URL"
  }
}

variable "mistral_model" {
  description = "Mistral AI model to use"
  type        = string
  default     = "mistral-large-2512"
}

variable "ledger_bucket" {
  description = "Object Storage bucket for the ledger of published posts (globally unique name)"
  type        = string
  default     = "uac-socializer-ledger"
}

variable "critic_model" {
  description = "Mistral AI model that checks every draft against its post"
  type        = string
  default     = "ministral-8b-2512"
}

variable "mistral_max_tokens" {
  description = "Maximum tokens for Mistral AI responses"
  type        = number
  default     = 1024
  validation {
    condition     = var.mistral_max_tokens > 0 && var.mistral_max_tokens <= 4096
    error_message = "mistral_max_tokens must be between 1 and 4096"
  }
}

variable "mistral_temperature" {
  description = "Mistral AI temperature (0.0-1.0, higher = more creative)"
  type        = number
  default     = 0.7
  validation {
    condition     = var.mistral_temperature >= 0.0 && var.mistral_temperature <= 1.0
    error_message = "mistral_temperature must be between 0.0 and 1.0"
  }
}

variable "ai_retry_count" {
  description = "Number of retries for AI generation failures"
  type        = number
  default     = 3
  validation {
    condition     = var.ai_retry_count > 0 && var.ai_retry_count <= 10
    error_message = "ai_retry_count must be between 1 and 10"
  }
}

variable "ai_timeout" {
  description = "Timeout for AI generation requests (e.g., 30s, 1m)"
  type        = string
  default     = "30s"
  validation {
    condition     = can(regex("^[0-9]+[sm]$", var.ai_timeout))
    error_message = "ai_timeout must be in Go duration format (e.g., 30s, 1m)"
  }
}

# Optional: the archive the bot reads its posts from
variable "site_base_url" {
  description = "Base URL of the site whose index.json and post JSON the bot reads"
  type        = string
  default     = "https://www.urbanartcore.com"
}

# Optional: Data paths (usually fixed in container)

variable "prompts_path" {
  description = "Path to prompt templates in container"
  type        = string
  default     = "/prompts"
}

# Environment (always production in this configuration)
variable "environment" {
  description = "Environment (production only)"
  type        = string
  default     = "production"
  validation {
    condition     = var.environment == "production"
    error_message = "Only production environment is supported"
  }
}

# Tags
variable "common_tags" {
  description = "Common tags applied to all resources"
  type        = map(string)
  default = {
    opentofu = "true"
    project  = "uac-socializer"
    stage    = "production"
  }
}
