variable "name" {
  description = "Load balancer name"
  type        = string
}

variable "vpc_id" {
  description = "VPC ID"
  type        = string
}

variable "subnet_ids" {
  description = "Subnet IDs for the ALB"
  type        = list(string)
}

variable "certificate_domain" {
  description = "Domain for ACM certificate (TLS)"
  type        = string
}
