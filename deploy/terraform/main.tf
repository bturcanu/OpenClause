terraform {
  required_version = ">= 1.0"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 5.0"
    }
  }
}

provider "aws" {
  region = var.region
}

# EKS cluster
module "cluster" {
  source = "./modules/cluster"

  cluster_name       = "${var.project_name}-${var.environment}"
  subnet_ids         = var.subnet_ids
  node_instance_type = var.node_instance_type
  desired_nodes      = var.desired_nodes
}

# RDS PostgreSQL database
module "database" {
  source = "./modules/database"

  db_name                 = var.db_name
  db_username             = var.db_username
  db_password             = var.db_password
  subnet_ids              = var.subnet_ids
  vpc_security_group_ids  = var.vpc_security_group_ids
  instance_class          = var.db_instance_class
}

# S3 bucket for evidence archive
module "storage" {
  source = "./modules/storage"

  bucket_name = "${var.project_name}-${var.environment}-evidence-${data.aws_caller_identity.current.account_id}"
}

# Secrets Manager for DB password, API keys, connector tokens
module "secrets" {
  source = "./modules/secrets"

  project_name = var.project_name
  secrets      = var.secrets
}

# Application Load Balancer with HTTPS
module "loadbalancer" {
  source = "./modules/loadbalancer"

  name              = "${var.project_name}-${var.environment}"
  vpc_id            = var.vpc_id
  subnet_ids        = var.subnet_ids
  certificate_domain = var.certificate_domain
}

data "aws_caller_identity" "current" {}
