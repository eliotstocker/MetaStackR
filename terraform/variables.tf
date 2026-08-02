variable "aws_region" {
  type        = string
  description = "The AWS region to deploy metastackr infrastructure."
  default     = "us-east-1"
}

variable "database_url" {
  type        = string
  description = "The connection string to Neon or standard PostgreSQL database."
  sensitive   = true
}
