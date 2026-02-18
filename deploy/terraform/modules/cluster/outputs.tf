output "cluster_endpoint" {
  description = "EKS cluster API endpoint"
  value       = aws_eks_cluster.main.endpoint
}

output "cluster_name" {
  description = "EKS cluster name"
  value       = aws_eks_cluster.main.name
}

output "cluster_ca_cert" {
  description = "Base64-encoded CA certificate for the cluster"
  value       = aws_eks_cluster.main.certificate_authority[0].data
}
