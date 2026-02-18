resource "aws_secretsmanager_secret" "secrets" {
  for_each = var.secrets

  name        = "${var.project_name}/${each.key}"
  description = "Secret: ${each.key}"

  tags = {
    Name = "${var.project_name}-${each.key}"
  }
}

resource "aws_secretsmanager_secret_version" "secrets" {
  for_each = var.secrets

  secret_id     = aws_secretsmanager_secret.secrets[each.key].id
  secret_string = each.value
}
