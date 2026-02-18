resource "aws_db_subnet_group" "main" {
  name       = "${var.db_name}-subnet-group"
  subnet_ids = var.subnet_ids

  tags = {
    Name = "${var.db_name}-subnet-group"
  }
}

resource "aws_db_instance" "main" {
  identifier     = var.db_name
  engine         = "postgres"
  engine_version = "16"

  db_name  = var.db_name
  username = var.db_username
  password = var.db_password

  instance_class        = var.instance_class
  allocated_storage     = 20
  storage_type          = "gp3"
  storage_encrypted     = true
  publicly_accessible   = false
  multi_az              = false

  db_subnet_group_name   = aws_db_subnet_group.main.name
  vpc_security_group_ids = var.vpc_security_group_ids

  skip_final_snapshot       = true
  deletion_protection       = false
  backup_retention_period   = 7
  backup_window             = "03:00-04:00"
  maintenance_window        = "sun:04:00-sun:05:00"

  tags = {
    Name = var.db_name
  }
}
