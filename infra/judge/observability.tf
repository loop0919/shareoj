variable "alerts_enabled" {
  description = "Enable alarm actions only after Discord and telemetry are verified. Disable for planned maintenance."
  type        = bool
  default     = false
}
variable "discord_webhook_secret_arn" {
  description = "Existing Secrets Manager secret containing only the Discord webhook URL. Never pass the URL to Terraform."
  type        = string
  default     = ""
}
variable "discord_webhook_secret_key" {
  description = "JSON key inside the existing secret; empty for a plain webhook URL."
  type        = string
  default     = ""
}
variable "notify_package_path" {
  type    = string
  default = "../../api/.build/judge-notify.zip"
}

locals {
  metrics_namespace = "Judge/${local.name}"
  worker_log_group  = "/judge/${local.name}/worker"
  logs_url          = "https://${var.aws_region}.console.aws.amazon.com/cloudwatch/home?region=${var.aws_region}#logsV2:logs-insights"
  alarm_actions     = var.discord_webhook_secret_arn == "" ? [] : [aws_sns_topic.alerts.arn]
  agent_config = {
    agent = { metrics_collection_interval = 60, region = var.aws_region, omit_hostname = true }
    metrics = {
      namespace              = local.metrics_namespace
      endpoint_override      = "https://monitoring.${var.aws_region}.api.aws"
      aggregation_dimensions = [[], ["Worker"]]
      metrics_collected = {
        mem      = { measurement = ["mem_used_percent"] }
        disk     = { resources = ["/"], measurement = ["used_percent"], drop_device = true, drop_original_metrics = ["disk_used_percent"] }
        procstat = [{ pattern = "^/usr/bin/python3 /opt/judge/worker.py$", measurement = ["pid_count"], append_dimensions = { Worker = "${local.name}-judge-worker" } }]
      }
    }
    logs = {
      endpoint_override    = "https://logs.${var.aws_region}.api.aws"
      force_flush_interval = 5
      logs_collected = { files = { collect_list = [{
        file_path       = "/var/log/judge/worker.jsonl"
        log_group_name  = local.worker_log_group
        log_stream_name = "${local.name}-judge-worker"
        timezone        = "UTC"
      }] } }
    }
  }
}
resource "aws_cloudwatch_log_group" "worker" {
  name              = local.worker_log_group
  retention_in_days = 14
}
locals {
  observability_statements = [
    { Effect = "Allow", Action = ["logs:CreateLogStream", "logs:PutLogEvents"], Resource = "${aws_cloudwatch_log_group.worker.arn}:*" },
    { Effect = "Allow", Action = ["cloudwatch:PutMetricData"], Resource = "*", Condition = { StringEquals = { "cloudwatch:namespace" = local.metrics_namespace } } }
  ]
  # Every host has its own process heartbeat. A stopped burst host is expected to report nothing.
  worker_alarms = merge(
    { for worker in aws_lightsail_instance.worker : worker.name => "breaching" },
    { for name, host in local.pool_hosts : name => host.role == "primary" ? "breaching" : "notBreaching" },
  )
}
resource "aws_iam_user_policy" "worker_observability" {
  user   = aws_iam_user.worker.name
  name   = "observability"
  policy = jsonencode({ Version = "2012-10-17", Statement = local.observability_statements })
}
resource "aws_cloudwatch_log_metric_filter" "errors" {
  for_each = {
    worker_platform = { group = aws_cloudwatch_log_group.worker.name, metric = "PlatformErrors", pattern = "{ $.event = \"failure\" && ($.category = \"platform\" || $.category = \"unknown\") }" }
    bridge_platform = { group = aws_cloudwatch_log_group.bridge.name, metric = "PlatformErrors", pattern = "{ $.event = \"failure\" && ($.category = \"platform\" || $.category = \"unknown\") }" }
    judge_code      = { group = aws_cloudwatch_log_group.worker.name, metric = "JudgeCodeErrors", pattern = "{ $.event = \"failure\" && $.category = \"judge_code\" }" }
  }
  name           = each.key
  log_group_name = each.value.group
  pattern        = each.value.pattern
  metric_transformation {
    name          = each.value.metric
    namespace     = local.metrics_namespace
    value         = "1"
    default_value = 0
  }
}
resource "aws_cloudwatch_log_metric_filter" "pending_age" {
  name           = "pending-age"
  log_group_name = aws_cloudwatch_log_group.bridge.name
  pattern        = "{ $.event = \"pending_age\" && $.oldestPendingSeconds = * }"
  metric_transformation {
    name      = "OldestPendingSeconds"
    namespace = local.metrics_namespace
    value     = "$.oldestPendingSeconds"
    unit      = "Seconds"
  }
}
resource "aws_cloudwatch_metric_alarm" "judge" {
  for_each = {
    pending          = { metric = "OldestPendingSeconds", comparison = "GreaterThanThreshold", threshold = 300, periods = 1, period = 60, statistic = "Maximum", missing = "notBreaching" }
    dispatch-missing = { metric = "OldestPendingSeconds", comparison = "LessThanThreshold", threshold = 0, periods = 3, period = 60, statistic = "Minimum", missing = "breaching" }
    platform         = { metric = "PlatformErrors", comparison = "GreaterThanThreshold", threshold = 0, periods = 1, period = 300, statistic = "Sum", missing = "notBreaching" }
    judge-code       = { metric = "JudgeCodeErrors", comparison = "GreaterThanThreshold", threshold = 0, periods = 1, period = 300, statistic = "Sum", missing = "notBreaching" }
    memory           = { metric = "mem_used_percent", comparison = "GreaterThanOrEqualToThreshold", threshold = 90, periods = 5, period = 60, statistic = "Average", missing = "notBreaching" }
    disk             = { metric = "disk_used_percent", comparison = "GreaterThanOrEqualToThreshold", threshold = 85, periods = 5, period = 60, statistic = "Average", missing = "notBreaching" }
  }
  alarm_name          = "${local.name}-judge-${each.key}"
  alarm_description   = "See docs/judge/observability.md; ${local.logs_url}"
  namespace           = local.metrics_namespace
  metric_name         = each.value.metric
  dimensions          = {}
  comparison_operator = each.value.comparison
  threshold           = each.value.threshold
  evaluation_periods  = each.value.periods
  datapoints_to_alarm = each.value.periods
  period              = each.value.period
  statistic           = each.value.statistic
  treat_missing_data  = each.value.missing
  actions_enabled     = var.alerts_enabled
  alarm_actions       = local.alarm_actions
  ok_actions          = local.alarm_actions
}
resource "aws_cloudwatch_metric_alarm" "worker" {
  for_each            = local.worker_alarms
  alarm_name          = each.key
  alarm_description   = "See docs/judge/observability.md; ${local.logs_url}"
  namespace           = local.metrics_namespace
  metric_name         = "procstat_lookup_pid_count"
  dimensions          = { Worker = each.key }
  comparison_operator = "LessThanThreshold"
  threshold           = 1
  evaluation_periods  = 3
  datapoints_to_alarm = 3
  period              = 60
  statistic           = "Maximum"
  treat_missing_data  = each.value
  actions_enabled     = var.alerts_enabled
  alarm_actions       = local.alarm_actions
  ok_actions          = local.alarm_actions
}
moved {
  from = aws_cloudwatch_metric_alarm.judge["worker"]
  to   = aws_cloudwatch_metric_alarm.worker["judge-dev-judge-worker"]
}
moved {
  from = aws_cloudwatch_metric_alarm.additional_worker[0]
  to   = aws_cloudwatch_metric_alarm.worker["judge-dev-judge-worker-2"]
}
resource "aws_cloudwatch_metric_alarm" "bridge_failure" {
  alarm_name          = "${local.name}-judge-bridge-failure"
  comparison_operator = "GreaterThanThreshold"
  threshold           = 0
  evaluation_periods  = 1
  treat_missing_data  = "notBreaching"
  actions_enabled     = var.alerts_enabled
  alarm_actions       = local.alarm_actions
  ok_actions          = local.alarm_actions
  metric_query {
    id          = "failures"
    expression  = "errors + throttles"
    return_data = true
  }
  dynamic "metric_query" {
    for_each = { errors = "Errors", throttles = "Throttles" }
    content {
      id = metric_query.key
      metric {
        namespace   = "AWS/Lambda"
        metric_name = metric_query.value
        dimensions  = { FunctionName = aws_lambda_function.bridge.function_name }
        period      = 60
        stat        = "Sum"
      }
    }
  }
}
resource "aws_sns_topic" "alerts" { name = "${local.name}-judge-alerts" }
resource "aws_sns_topic_policy" "alerts" {
  arn = aws_sns_topic.alerts.arn
  policy = jsonencode({ Version = "2012-10-17", Statement = [{
    Effect    = "Allow", Principal = { Service = "cloudwatch.amazonaws.com" }, Action = "sns:Publish", Resource = aws_sns_topic.alerts.arn
    Condition = { StringEquals = { "aws:SourceAccount" = split(":", aws_sns_topic.alerts.arn)[4] }, ArnLike = { "aws:SourceArn" = "arn:aws:cloudwatch:${var.aws_region}:${split(":", aws_sns_topic.alerts.arn)[4]}:alarm:${local.name}-judge-*" } }
  }] })
}
resource "aws_sqs_queue" "notification_dead" {
  name                      = "${local.name}-judge-notification-dead"
  sqs_managed_sse_enabled   = true
  message_retention_seconds = 1209600
}
resource "aws_sqs_queue_policy" "notification_dead" {
  queue_url = aws_sqs_queue.notification_dead.id
  policy = jsonencode({ Version = "2012-10-17", Statement = [{
    Effect    = "Allow", Principal = { Service = "sns.amazonaws.com" }, Action = "sqs:SendMessage", Resource = aws_sqs_queue.notification_dead.arn,
    Condition = { ArnEquals = { "aws:SourceArn" = aws_sns_topic.alerts.arn } }
  }] })
}
resource "aws_cloudwatch_log_group" "notify" {
  name              = "/aws/lambda/${local.name}-judge-notify"
  retention_in_days = 14
}
resource "aws_iam_role" "notify" {
  name               = "${local.name}-judge-notify"
  assume_role_policy = jsonencode({ Version = "2012-10-17", Statement = [{ Effect = "Allow", Principal = { Service = "lambda.amazonaws.com" }, Action = "sts:AssumeRole" }] })
}
resource "aws_iam_role_policy" "notify" {
  role = aws_iam_role.notify.id
  policy = jsonencode({ Version = "2012-10-17", Statement = concat([
    { Effect = "Allow", Action = ["logs:CreateLogStream", "logs:PutLogEvents"], Resource = "${aws_cloudwatch_log_group.notify.arn}:*" },
    { Effect = "Allow", Action = ["sqs:SendMessage"], Resource = aws_sqs_queue.notification_dead.arn }
    ], var.discord_webhook_secret_arn == "" ? [] : [
    { Effect = "Allow", Action = ["secretsmanager:GetSecretValue"], Resource = var.discord_webhook_secret_arn }
  ]) })
}
resource "aws_lambda_function" "notify" {
  function_name    = "${local.name}-judge-notify"
  role             = aws_iam_role.notify.arn
  runtime          = "python3.13"
  architectures    = ["arm64"]
  handler          = "notify.handler"
  filename         = var.notify_package_path
  source_code_hash = filebase64sha256(var.notify_package_path)
  memory_size      = 128
  timeout          = 60
  dead_letter_config { target_arn = aws_sqs_queue.notification_dead.arn }
  environment {
    variables = {
      WEBHOOK_SECRET_ARN = var.discord_webhook_secret_arn
      WEBHOOK_SECRET_KEY = var.discord_webhook_secret_key
      ENVIRONMENT        = var.environment
      LOGS_URL           = local.logs_url
    }
  }
  lifecycle {
    precondition {
      condition     = !var.alerts_enabled || var.discord_webhook_secret_arn != ""
      error_message = "Register a Discord webhook secret before enabling alerts."
    }
  }
  depends_on = [aws_iam_role_policy.notify]
}
resource "aws_lambda_function_event_invoke_config" "notify" {
  function_name                = aws_lambda_function.notify.function_name
  maximum_event_age_in_seconds = 3600
  maximum_retry_attempts       = 2
}
resource "aws_lambda_permission" "notify" {
  statement_id  = "SNSAlerts"
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.notify.function_name
  principal     = "sns.amazonaws.com"
  source_arn    = aws_sns_topic.alerts.arn
}
resource "aws_sns_topic_subscription" "notify" {
  topic_arn      = aws_sns_topic.alerts.arn
  protocol       = "lambda"
  endpoint       = aws_lambda_function.notify.arn
  redrive_policy = jsonencode({ deadLetterTargetArn = aws_sqs_queue.notification_dead.arn })
  depends_on     = [aws_lambda_permission.notify, aws_sqs_queue_policy.notification_dead]
}
output "cloudwatch_agent_config" { value = jsonencode(local.agent_config) }
output "cloudwatch_agent_configs" {
  value = { for name in keys(local.worker_alarms) : name => replace(jsonencode(local.agent_config), "${local.name}-judge-worker", name) }
}
output "alerts_topic_arn" { value = aws_sns_topic.alerts.arn }
output "notification_dead_queue_url" { value = aws_sqs_queue.notification_dead.url }
resource "aws_cloudwatch_query_definition" "failures" {
  name            = "${local.name}/judge-failures"
  log_group_names = [aws_cloudwatch_log_group.worker.name, aws_cloudwatch_log_group.bridge.name]
  query_string    = <<-QUERY
    fields @timestamp, service, event, category, reason, submissionId, attemptId, verdict, durationMs
    | filter event = "failure"
    | sort @timestamp desc
    | limit 100
  QUERY
}
