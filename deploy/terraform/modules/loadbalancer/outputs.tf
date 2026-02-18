output "lb_dns_name" {
  description = "ALB DNS name"
  value       = aws_lb.main.dns_name
}

output "lb_arn" {
  description = "ALB ARN"
  value       = aws_lb.main.arn
}
