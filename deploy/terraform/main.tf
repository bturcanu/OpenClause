terraform {
  required_version = ">= 1.0"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 5.0"
    }
  }

  # Remote backend for state locking and team collaboration.
  # Uncomment and configure with your own S3 bucket and DynamoDB table:
  #
  # backend "s3" {
  #   bucket         = "your-terraform-state-bucket"
  #   key            = "openclause/terraform.tfstate"
  #   region         = "us-east-1"
  #   dynamodb_table = "terraform-locks"
  #   encrypt        = true
  # }
}

provider "aws" {
  region = var.region
}

# EKS cluster
module "cluster" {
  source = "./modules/cluster"

  cluster_name        = "${var.project_name}-${var.environment}"
  subnet_ids          = var.subnet_ids
  node_instance_type  = var.node_instance_type
  desired_nodes       = var.desired_nodes
  public_access_cidrs = var.eks_public_access_cidrs
}

# RDS PostgreSQL database
module "database" {
  source = "./modules/database"

  db_name                = var.db_name
  db_username            = var.db_username
  db_password            = var.db_password
  subnet_ids             = var.subnet_ids
  vpc_security_group_ids = var.vpc_security_group_ids
  instance_class         = var.db_instance_class
  multi_az               = var.environment != "dev"
  deletion_protection    = var.environment != "dev"
  skip_final_snapshot    = var.environment == "dev"
}

# S3 bucket for evidence archive
module "storage" {
  source = "./modules/storage"

  bucket_name = "${var.project_name}-${var.environment}-evidence-${data.aws_caller_identity.current.account_id}"
  kms_key_arn = var.kms_key_arn
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

  name               = "${var.project_name}-${var.environment}"
  vpc_id             = var.vpc_id
  subnet_ids         = var.subnet_ids
  certificate_domain = var.certificate_domain
}

data "aws_caller_identity" "current" {}
