variable "project" {
  type = string
}

variable "vpc_id" {
  type = string
}

variable "subnet_ids" {
  type = list(string)
}

variable "allowed_security_group_id" {
  type = string
}

variable "auth_token" {
  type      = string
  sensitive = true
}

resource "aws_elasticache_subnet_group" "main" {
  name       = "${var.project}-redis"
  subnet_ids = var.subnet_ids
}

resource "aws_elasticache_replication_group" "main" {
  replication_group_id       = "${var.project}-redis"
  description                = "Entropy session and cache data"
  engine                     = "redis"
  engine_version             = "7.1"
  node_type                  = "cache.t3.micro"
  num_cache_clusters         = 2
  parameter_group_name       = "default.redis7"
  port                       = 6379
  subnet_group_name          = aws_elasticache_subnet_group.main.name
  security_group_ids         = [aws_security_group.redis.id]
  automatic_failover_enabled = true
  multi_az_enabled           = true
  at_rest_encryption_enabled = true
  transit_encryption_enabled = true
  auth_token                 = var.auth_token
  apply_immediately          = true
}

resource "aws_security_group" "redis" {
  name   = "${var.project}-redis"
  vpc_id = var.vpc_id
  ingress {
    from_port       = 6379
    to_port         = 6379
    protocol        = "tcp"
    security_groups = [var.allowed_security_group_id]
  }
  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

output "endpoint" {
  value = aws_elasticache_replication_group.main.primary_endpoint_address
}
