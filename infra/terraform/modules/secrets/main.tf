variable "project" {
  type = string
}

variable "db_host" {
  type = string
}

variable "db_password" {
  type      = string
  sensitive = true
}

variable "redis_host" {
  type = string
}

variable "redis_token" {
  type      = string
  sensitive = true
}

resource "aws_secretsmanager_secret" "database_url" {
  name                    = "${var.project}/database-url"
  recovery_window_in_days = 7
}

resource "aws_secretsmanager_secret_version" "database_url" {
  secret_id     = aws_secretsmanager_secret.database_url.id
  secret_string = "postgres://entropy:${var.db_password}@${var.db_host}:5432/entropy?sslmode=require"
}

resource "aws_secretsmanager_secret" "redis_url" {
  name                    = "${var.project}/redis-url"
  recovery_window_in_days = 7
}

resource "aws_secretsmanager_secret_version" "redis_url" {
  secret_id     = aws_secretsmanager_secret.redis_url.id
  secret_string = "rediss://default:${var.redis_token}@${var.redis_host}:6379/0"
}

output "secret_arns" {
  value = {
    database_url = aws_secretsmanager_secret.database_url.arn
    redis_url    = aws_secretsmanager_secret.redis_url.arn
  }
}
