variable "project" {
  type = string
}

variable "services" {
  type = set(string)
}

resource "aws_ecr_repository" "service" {
  for_each = var.services

  name                 = "${var.project}-${each.value}"
  image_tag_mutability = "IMMUTABLE"

  image_scanning_configuration {
    scan_on_push = true
  }

  encryption_configuration {
    encryption_type = "AES256"
  }
}

resource "aws_ecr_lifecycle_policy" "service" {
  for_each   = aws_ecr_repository.service
  repository = each.value.name

  policy = jsonencode({
    rules = [{
      rulePriority = 1
      description  = "Retain the 20 newest images"
      selection = {
        tagStatus   = "any"
        countType   = "imageCountMoreThan"
        countNumber = 20
      }
      action = { type = "expire" }
    }]
  })
}

output "repository_urls" {
  value = { for name, repository in aws_ecr_repository.service : name => repository.repository_url }
}
