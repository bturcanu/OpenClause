variable "project_name" {
  description = "Project name prefix for secret names"
  type        = string
}

variable "secrets" {
  description = "Map of secret names to values (e.g. db_password, api_keys, connector_tokens)"
  type        = map(string)
  sensitive   = true
}
