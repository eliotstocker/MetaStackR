variable "aws_region" {
  type        = string
  description = "The AWS region to deploy metastackr infrastructure."
  default     = "eu-west-1"
}

variable "database_url" {
  type        = string
  description = "The connection string to Neon or standard PostgreSQL database."
  sensitive   = true
}

variable "github_app_id" {
  type        = string
  description = "GitHub App ID for S2S Authentication."
  default     = ""
}

variable "github_private_key" {
  type        = string
  description = "GitHub App RSA Private Key in PEM format."
  default     = ""
  sensitive   = true
}

variable "gh_token" {
  type        = string
  description = "GitHub PAT fallback token."
  default     = ""
  sensitive   = true
}

variable "webhook_secret" {
  type        = string
  description = "GitHub Webhook Secret for HMAC verification."
  default     = ""
  sensitive   = true
}
