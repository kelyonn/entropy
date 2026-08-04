variable "project" {
  type = string
}

variable "oidc_arn" {
  type = string
}

variable "oidc_url" {
  type = string
}

variable "s3_bucket_arn" {
  type = string
}

variable "secret_arns" {
  type = list(string)
}

locals {
  oidc_host = replace(var.oidc_url, "https://", "")
}

resource "aws_iam_role" "session_manager" {
  name = "${var.project}-session-manager"
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect = "Allow"
      Principal = {
        Federated = var.oidc_arn
      }
      Action = "sts:AssumeRoleWithWebIdentity"
      Condition = {
        StringEquals = {
          "${local.oidc_host}:aud" = "sts.amazonaws.com"
          "${local.oidc_host}:sub" = "system:serviceaccount:entropy:session-manager"
        }
      }
    }]
  })
}

resource "aws_iam_role_policy" "session_manager_s3" {
  name = "s3-snapshots"
  role = aws_iam_role.session_manager.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = ["s3:GetObject", "s3:PutObject"]
      Resource = "${var.s3_bucket_arn}/*"
    }]
  })
}

resource "aws_iam_role_policy" "session_manager_secrets" {
  name = "read-runtime-secrets"
  role = aws_iam_role.session_manager.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = ["secretsmanager:GetSecretValue"]
      Resource = var.secret_arns
    }]
  })
}

resource "aws_iam_role_policy" "session_manager_metrics" {
  name = "publish-cloudwatch-metrics"
  role = aws_iam_role.session_manager.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = ["cloudwatch:PutMetricData"]
      Resource = "*"
      Condition = {
        StringEquals = {
          "cloudwatch:namespace" = "Entropy/Production"
        }
      }
    }]
  })
}

resource "aws_iam_role" "ai_explainer" {
  name = "${var.project}-ai-explainer"
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect = "Allow"
      Principal = {
        Federated = var.oidc_arn
      }
      Action = "sts:AssumeRoleWithWebIdentity"
      Condition = {
        StringEquals = {
          "${local.oidc_host}:aud" = "sts.amazonaws.com"
          "${local.oidc_host}:sub" = "system:serviceaccount:entropy:ai-explainer"
        }
      }
    }]
  })
}

resource "aws_iam_role_policy" "ai_bedrock" {
  name = "bedrock-invoke"
  role = aws_iam_role.ai_explainer.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = ["bedrock:InvokeModel", "bedrock:InvokeModelWithResponseStream"]
      Resource = "*"
    }]
  })
}

resource "aws_iam_role" "load_balancer_controller" {
  name = "${var.project}-aws-load-balancer-controller"
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect = "Allow"
      Principal = {
        Federated = var.oidc_arn
      }
      Action = "sts:AssumeRoleWithWebIdentity"
      Condition = {
        StringEquals = {
          "${local.oidc_host}:aud" = "sts.amazonaws.com"
          "${local.oidc_host}:sub" = "system:serviceaccount:kube-system:aws-load-balancer-controller"
        }
      }
    }]
  })
}

resource "aws_iam_role_policy" "load_balancer_controller" {
  name = "manage-application-load-balancers"
  role = aws_iam_role.load_balancer_controller.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "iam:CreateServiceLinkedRole",
          "ec2:Describe*",
          "ec2:CreateSecurityGroup",
          "ec2:DeleteSecurityGroup",
          "ec2:AuthorizeSecurityGroupIngress",
          "ec2:RevokeSecurityGroupIngress",
          "ec2:CreateTags",
          "ec2:DeleteTags",
          "elasticloadbalancing:*",
          "acm:ListCertificates",
          "acm:DescribeCertificate",
          "cognito-idp:DescribeUserPoolClient",
          "wafv2:GetWebACL",
          "wafv2:GetWebACLForResource",
          "wafv2:AssociateWebACL",
          "wafv2:DisassociateWebACL",
          "shield:GetSubscriptionState",
          "shield:DescribeProtection",
          "shield:CreateProtection",
          "shield:DeleteProtection"
        ]
        Resource = "*"
      }
    ]
  })
}

resource "aws_iam_role" "fluent_bit" {
  name = "${var.project}-fluent-bit"
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect = "Allow"
      Principal = {
        Federated = var.oidc_arn
      }
      Action = "sts:AssumeRoleWithWebIdentity"
      Condition = {
        StringEquals = {
          "${local.oidc_host}:aud" = "sts.amazonaws.com"
          "${local.oidc_host}:sub" = "system:serviceaccount:amazon-cloudwatch:aws-for-fluent-bit"
        }
      }
    }]
  })
}

resource "aws_iam_role_policy" "fluent_bit" {
  name = "write-cloudwatch-logs"
  role = aws_iam_role.fluent_bit.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect = "Allow"
      Action = [
        "logs:CreateLogStream",
        "logs:DescribeLogStreams",
        "logs:PutLogEvents"
      ]
      Resource = "arn:aws:logs:*:*:log-group:/aws/eks/${var.project}/*"
    }]
  })
}

output "session_manager_role_arn" {
  value = aws_iam_role.session_manager.arn
}

output "ai_explainer_role_arn" {
  value = aws_iam_role.ai_explainer.arn
}

output "load_balancer_controller_role_arn" {
  value = aws_iam_role.load_balancer_controller.arn
}

output "fluent_bit_role_arn" {
  value = aws_iam_role.fluent_bit.arn
}
