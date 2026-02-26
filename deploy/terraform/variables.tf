variable "project_name" {
  description = "Project name prefix for resources"
  type        = string
  default     = "openclause"
}

variable "environment" {
  description = "Environment (e.g. dev, staging, prod)"
  type        = string
}

variable "region" {
  description = "AWS region"
  type        = string
}

variable "db_instance_class" {
  description = "RDS instance class"
  type        = string
}

variable "db_name" {
  description = "Database name"
  type        = string
}

variable "db_username" {
  description = "Database master username"
  type        = string
}

variable "db_password" {
  description = "Database master password"
  type        = string
  sensitive   = true
}

variable "vpc_id" {
  description = "VPC ID for load balancer and networking"
  type        = string
}

variable "subnet_ids" {
  description = "List of subnet IDs for EKS, RDS, and ALB"
  type        = list(string)
}

variable "vpc_security_group_ids" {
  description = "Security group IDs for RDS access"
  type        = list(string)
}

variable "node_instance_type" {
  description = "EKS node group instance type"
  type        = string
  default     = "t3.medium"
}

variable "desired_nodes" {
  description = "Desired number of EKS nodes"
  type        = number
  default     = 2
}

variable "certificate_domain" {
  description = "Domain for ACM certificate (TLS)"
  type        = string
}

variable "secrets" {
  description = "Map of secret names to values (DB password, API keys, connector tokens)"
  type        = map(string)
  sensitive   = true
}

variable "eks_public_access_cidrs" {
  description = "CIDR blocks allowed to access the EKS API server public endpoint"
  type        = list(string)
  default     = []
}

variable "kms_key_arn" {
  description = "Optional KMS key ARN for S3 evidence encryption"
  type        = string
  default     = ""
}
