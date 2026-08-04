terraform {
  required_version = ">= 1.5"
  backend "s3" {
    encrypt = true
  }
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
    random = {
      source  = "hashicorp/random"
      version = "~> 3.6"
    }
    tls = {
      source  = "hashicorp/tls"
      version = "~> 4.0"
    }
  }
}

provider "aws" {
  region = var.aws_region
}

variable "aws_region" {
  type    = string
  default = "us-east-1"
}

variable "project" {
  type    = string
  default = "entropy"
}

variable "vpc_cidr" {
  type    = string
  default = "10.0.0.0/16"
}

variable "alarm_email" {
  type        = string
  default     = ""
  description = "Optional email endpoint for production alarm notifications."
}

module "vpc" {
  source  = "./modules/vpc"
  project = var.project
  cidr    = var.vpc_cidr
}

module "eks" {
  source     = "./modules/eks"
  project    = var.project
  vpc_id     = module.vpc.vpc_id
  subnet_ids = module.vpc.private_subnet_ids
}

resource "random_password" "database" {
  length  = 32
  special = false
}

resource "random_password" "redis" {
  length  = 32
  special = false
}

module "rds" {
  source                    = "./modules/rds"
  project                   = var.project
  vpc_id                    = module.vpc.vpc_id
  subnet_ids                = module.vpc.private_subnet_ids
  allowed_security_group_id = module.eks.cluster_security_group_id
  password                  = random_password.database.result
}

module "redis" {
  source                    = "./modules/redis"
  project                   = var.project
  vpc_id                    = module.vpc.vpc_id
  subnet_ids                = module.vpc.private_subnet_ids
  allowed_security_group_id = module.eks.cluster_security_group_id
  auth_token                = random_password.redis.result
}

module "s3" {
  source  = "./modules/s3"
  project = var.project
}

module "iam" {
  source        = "./modules/iam"
  project       = var.project
  oidc_arn      = module.eks.oidc_provider_arn
  oidc_url      = module.eks.oidc_provider_url
  s3_bucket_arn = module.s3.bucket_arn
  secret_arns   = values(module.secrets.secret_arns)
}

module "cognito" {
  source  = "./modules/cognito"
  project = var.project
}

module "secrets" {
  source      = "./modules/secrets"
  project     = var.project
  db_host     = module.rds.endpoint
  db_password = random_password.database.result
  redis_host  = module.redis.endpoint
  redis_token = random_password.redis.result
}

module "ecr" {
  source   = "./modules/ecr"
  project  = var.project
  services = ["session-manager", "airlock", "ai-explainer", "ebpf-monitor"]
}

module "monitoring" {
  source      = "./modules/monitoring"
  project     = var.project
  alarm_email = var.alarm_email
}

output "eks_cluster_name" {
  value = module.eks.cluster_name
}

output "rds_endpoint" {
  value = module.rds.endpoint
}

output "redis_endpoint" {
  value = module.redis.endpoint
}

output "s3_bucket" {
  value = module.s3.bucket_name
}

output "cognito_user_pool_id" {
  value = module.cognito.user_pool_id
}

output "cognito_client_id" {
  value = module.cognito.client_id
}

output "secret_arns" {
  value = module.secrets.secret_arns
}

output "ecr_repository_urls" {
  value = module.ecr.repository_urls
}

output "service_role_arns" {
  value = {
    session_manager          = module.iam.session_manager_role_arn
    ai_explainer             = module.iam.ai_explainer_role_arn
    load_balancer_controller = module.iam.load_balancer_controller_role_arn
    fluent_bit               = module.iam.fluent_bit_role_arn
  }
}

output "alerts_topic_arn" {
  value = module.monitoring.alerts_topic_arn
}
