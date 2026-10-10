resource "aws_cloudwatch_log_group" "bridge" {
  name              = "/aws/lambda/${local.name}-judge-bridge"
  retention_in_days = 14
}
resource "aws_iam_role" "bridge" {
  name = "${local.name}-judge-bridge"
  assume_role_policy = jsonencode({
    Version   = "2012-10-17"
    Statement = [{ Effect = "Allow", Principal = { Service = "lambda.amazonaws.com" }, Action = "sts:AssumeRole" }]
  })
}
resource "aws_iam_role_policy" "bridge" {
  role = aws_iam_role.bridge.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      { Effect = "Allow", Action = ["logs:CreateLogStream", "logs:PutLogEvents"], Resource = "${aws_cloudwatch_log_group.bridge.arn}:*" },
      { Effect = "Allow", Action = ["secretsmanager:GetSecretValue"], Resource = var.database.secret_arn },
      { Effect = "Allow", Action = ["s3:PutObject"], Resource = "${aws_s3_bucket.jobs.arn}/jobs/*" },
      { Effect = "Allow", Action = ["sqs:SendMessage"], Resource = aws_sqs_queue.queue["requests"].arn },
      { Effect = "Allow", Action = ["sqs:ReceiveMessage", "sqs:DeleteMessage", "sqs:GetQueueAttributes"], Resource = aws_sqs_queue.queue["results"].arn },
      { Effect = "Allow", Action = ["ec2:CreateNetworkInterface", "ec2:DescribeNetworkInterfaces", "ec2:DescribeSubnets", "ec2:DeleteNetworkInterface", "ec2:AssignPrivateIpAddresses", "ec2:UnassignPrivateIpAddresses"], Resource = "*" },
      # Capacity: start pool hosts and extend their contest hold; never stop, terminate or relabel them.
      { Effect = "Allow", Action = ["ec2:DescribeInstances"], Resource = "*" },
      { Effect = "Allow", Action = ["ec2:StartInstances"], Resource = "arn:aws:ec2:${var.aws_region}:*:instance/*", Condition = { StringEquals = { "aws:ResourceTag/JudgePool" = local.name } } },
      {
        Effect    = "Allow", Action = ["ec2:CreateTags"], Resource = "arn:aws:ec2:${var.aws_region}:*:instance/*",
        Condition = { StringEquals = { "aws:ResourceTag/JudgePool" = local.name }, "ForAllValues:StringEquals" = { "aws:TagKeys" = ["JudgeHoldUntil"] } }
      }
    ]
  })
}
resource "aws_lambda_function" "bridge" {
  function_name                  = "${local.name}-judge-bridge"
  role                           = aws_iam_role.bridge.arn
  handler                        = "bootstrap"
  runtime                        = "provided.al2023"
  architectures                  = ["arm64"]
  timeout                        = 120
  memory_size                    = 256
  s3_bucket                      = aws_s3_object.bridge.bucket
  s3_key                         = aws_s3_object.bridge.key
  s3_object_version              = aws_s3_object.bridge.version_id
  source_code_hash               = filebase64sha256(var.bridge_package_path)
  reserved_concurrent_executions = 5
  vpc_config {
    subnet_ids                  = var.database.subnet_ids
    security_group_ids          = [var.database.security_group_id]
    ipv6_allowed_for_dual_stack = true
  }
  environment {
    variables = {
      DATABASE_HOST                 = var.database.host
      DATABASE_NAME                 = var.database.name
      DATABASE_SECRET_ARN           = var.database.secret_arn
      AWS_USE_DUALSTACK_ENDPOINT    = "true"
      JUDGE_JOB_BUCKET              = aws_s3_bucket.jobs.id
      JUDGE_REQUEST_QUEUE_URL       = aws_sqs_queue.queue["requests"].url
      JUDGE_RUNTIME_DIGEST          = var.runtime_digest
      JUDGE_PREVIOUS_RUNTIME_DIGEST = var.previous_runtime_digest
      JUDGE_ENABLED_RUNTIMES        = join(",", var.enabled_runtimes)
      JUDGE_CAPACITY_ENABLED        = tostring(var.capacity_enabled)
      JUDGE_POOL                    = local.name
      JUDGE_BURST_MIN_PARTICIPANTS  = tostring(var.burst_min_participants)
    }
  }
  lifecycle {
    precondition {
      condition     = !var.enabled || var.runtime_digest != ""
      error_message = "Install and smoke-test the runtime before enabling dispatch."
    }
    precondition {
      condition     = !var.capacity_enabled || var.runtime_digest != ""
      error_message = "Tag the pool with a smoke-tested runtime before the bridge starts hosts."
    }
  }
  depends_on = [aws_iam_role_policy.bridge]
}
resource "aws_cloudwatch_event_rule" "dispatch" {
  name                = "${local.name}-judge-dispatch"
  schedule_expression = "rate(1 minute)"
  state               = var.enabled ? "ENABLED" : "DISABLED"
}
resource "aws_cloudwatch_event_target" "dispatch" {
  rule = aws_cloudwatch_event_rule.dispatch.name
  arn  = aws_lambda_function.bridge.arn
}
resource "aws_lambda_permission" "dispatch" {
  statement_id  = "EventBridgeDispatch"
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.bridge.function_name
  principal     = "events.amazonaws.com"
  source_arn    = aws_cloudwatch_event_rule.dispatch.arn
}
resource "aws_lambda_event_source_mapping" "results" {
  event_source_arn        = aws_sqs_queue.queue["results"].arn
  function_name           = aws_lambda_function.bridge.arn
  batch_size              = 1
  function_response_types = ["ReportBatchItemFailures"]
  enabled                 = var.enabled
}
resource "aws_cloudwatch_metric_alarm" "dead" {
  for_each            = aws_sqs_queue.dead
  alarm_name          = "${local.name}-judge-${each.key}-dead"
  comparison_operator = "GreaterThanThreshold"
  evaluation_periods  = 1
  metric_name         = "ApproximateNumberOfMessagesVisible"
  namespace           = "AWS/SQS"
  period              = 60
  statistic           = "Maximum"
  threshold           = 0
  treat_missing_data  = "notBreaching"
  dimensions          = { QueueName = each.value.name }
  actions_enabled     = var.alerts_enabled
  alarm_actions       = local.alarm_actions
  ok_actions          = local.alarm_actions
}
