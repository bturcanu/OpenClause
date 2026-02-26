variable "bucket_name" {
  description = "S3 bucket name for evidence archive"
  type        = string
}

variable "kms_key_arn" {
  description = "Optional KMS key ARN for S3 encryption. If empty, AES256 is used."
  type        = string
  default     = ""
}
