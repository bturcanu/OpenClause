output "cluster_endpoint" {
  description = "EKS cluster API endpoint"
  value       = module.cluster.cluster_endpoint
}

output "db_endpoint" {
  description = "RDS database endpoint"
  value       = module.database.db_endpoint
}

output "s3_bucket_name" {
  description = "S3 bucket name for evidence archive"
  value       = module.storage.bucket_name
}

output "load_balancer_dns" {
  description = "ALB DNS name"
  value       = module.loadbalancer.lb_dns_name
}
