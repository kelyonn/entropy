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

variable "password" {
  type      = string
  sensitive = true
}

resource "aws_db_subnet_group" "main" {
  name       = "${var.project}-db"
  subnet_ids = var.subnet_ids
}

resource "aws_db_instance" "main" {
  identifier              = "${var.project}-postgres"
  engine                  = "postgres"
  engine_version          = "15"
  instance_class          = "db.t3.medium"
  allocated_storage       = 20
  db_name                 = "entropy"
  username                = "entropy"
  password                = var.password
  multi_az                = true
  skip_final_snapshot     = true
  db_subnet_group_name    = aws_db_subnet_group.main.name
  vpc_security_group_ids  = [aws_security_group.db.id]
  storage_encrypted       = true
  backup_retention_period = 7
  deletion_protection     = true
  apply_immediately       = true
  publicly_accessible     = false
}

resource "aws_security_group" "db" {
  name   = "${var.project}-rds"
  vpc_id = var.vpc_id
  ingress {
    from_port       = 5432
    to_port         = 5432
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
  value = aws_db_instance.main.address
}
