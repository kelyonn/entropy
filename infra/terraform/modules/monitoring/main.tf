variable "project" {
  type = string
}

variable "alarm_email" {
  type    = string
  default = ""
}

resource "aws_sns_topic" "alerts" {
  name = "${var.project}-alerts"
}

resource "aws_sns_topic_subscription" "email" {
  count     = var.alarm_email == "" ? 0 : 1
  topic_arn = aws_sns_topic.alerts.arn
  protocol  = "email"
  endpoint  = var.alarm_email
}

locals {
  alarms = {
    active_sessions = {
      metric              = "ActiveSessions"
      threshold           = 95
      comparison_operator = "GreaterThanThreshold"
      evaluation_periods  = 2
      period              = 60
      statistic           = "Average"
    }
    error_rate = {
      metric              = "ErrorRate"
      threshold           = 5
      comparison_operator = "GreaterThanThreshold"
      evaluation_periods  = 5
      period              = 60
      statistic           = "Average"
    }
    session_creation_failures = {
      metric              = "SessionCreationFailures"
      threshold           = 3
      comparison_operator = "GreaterThanThreshold"
      evaluation_periods  = 1
      period              = 60
      statistic           = "Sum"
    }
  }
}

resource "aws_cloudwatch_metric_alarm" "standard" {
  for_each = local.alarms

  alarm_name          = "${var.project}-${replace(each.key, "_", "-")}"
  namespace           = "Entropy/Production"
  metric_name         = each.value.metric
  comparison_operator = each.value.comparison_operator
  evaluation_periods  = each.value.evaluation_periods
  period              = each.value.period
  statistic           = each.value.statistic
  threshold           = each.value.threshold
  treat_missing_data  = "notBreaching"
  alarm_actions       = [aws_sns_topic.alerts.arn]
  ok_actions          = [aws_sns_topic.alerts.arn]
}

resource "aws_cloudwatch_metric_alarm" "ai_latency" {
  alarm_name          = "${var.project}-ai-explanation-latency"
  namespace           = "Entropy/Production"
  metric_name         = "AIExplanationLatency"
  comparison_operator = "GreaterThanThreshold"
  evaluation_periods  = 2
  period              = 60
  extended_statistic  = "p99"
  threshold           = 2000
  treat_missing_data  = "notBreaching"
  alarm_actions       = [aws_sns_topic.alerts.arn]
  ok_actions          = [aws_sns_topic.alerts.arn]
}

resource "aws_cloudwatch_log_group" "service" {
  for_each = toset(["application", "session-manager", "airlock", "ai-explainer", "ebpf-monitor"])

  name              = "/aws/eks/${var.project}/${each.value}"
  retention_in_days = 30
}

output "alerts_topic_arn" {
  value = aws_sns_topic.alerts.arn
}
