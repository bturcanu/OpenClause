output "db_endpoint" {
  description = "RDS database endpoint (hostname:port)"
  value       = aws_db_instance.main.endpoint
}

output "db_name" {
  description = "Database name"
  value       = aws_db_instance.main.db_name
}
