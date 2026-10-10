locals {
  lambda_package_path = coalesce(var.lambda_package_path, "${path.module}/../../api/.build/api.zip")
}

data "aws_caller_identity" "current" {}
data "aws_partition" "current" {}

resource "aws_iam_role_policy" "judge_dispatch" {
  name = "judge-dispatch"
  role = aws_iam_role.api.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = ["lambda:InvokeFunction"]
      Resource = "arn:${data.aws_partition.current.partition}:lambda:${var.aws_region}:${data.aws_caller_identity.current.account_id}:function:${local.name}-judge-bridge"
    }]
  })
}

# Account settings read the sign-in method and delete the user. Google sign-ins lack the scope for the token-based calls.
resource "aws_iam_role_policy" "cognito_users" {
  name = "cognito-users"
  role = aws_iam_role.api.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = ["cognito-idp:AdminGetUser", "cognito-idp:AdminDeleteUser"]
      Resource = aws_cognito_user_pool.users.arn
    }]
  })
}

resource "aws_cloudwatch_log_group" "lambda" {
  name              = "/aws/lambda/${local.name}-api"
  retention_in_days = var.log_retention_days
}

resource "aws_iam_role" "api" {
  name = "judge-dev-api-ApiLambdaRole-8d6vqml9loum"
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Service = "lambda.amazonaws.com" }
      Action    = "sts:AssumeRole"
    }]
  })
}

resource "aws_iam_role_policy" "logs" {
  name = "cloudwatch-logs"
  role = aws_iam_role.api.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Sid      = "WriteFunctionLogs"
      Effect   = "Allow"
      Action   = ["logs:CreateLogStream", "logs:PutLogEvents"]
      Resource = ["${aws_cloudwatch_log_group.lambda.arn}:*", "${aws_cloudwatch_log_group.migration.arn}:*"]
    }]
  })
}

resource "aws_lambda_function" "api" {
  function_name                  = "${local.name}-api"
  description                    = "HTTP API for ${var.project_name}"
  architectures                  = ["arm64"]
  handler                        = "bootstrap"
  runtime                        = "provided.al2023"
  memory_size                    = var.lambda_memory_size
  timeout                        = 30
  role                           = aws_iam_role.api.arn
  s3_bucket                      = aws_s3_object.api_package.bucket
  s3_key                         = aws_s3_object.api_package.key
  s3_object_version              = aws_s3_object.api_package.version_id
  source_code_hash               = filebase64sha256(local.lambda_package_path)
  depends_on                     = [aws_iam_role_policy.logs, aws_iam_role_policy.database, aws_iam_role_policy.test_data, aws_iam_role_policy.judge_dispatch, aws_iam_role_policy.cognito_users, aws_iam_role_policy.vpc, aws_route_table_association.private]
  reserved_concurrent_executions = 20

  vpc_config {
    ipv6_allowed_for_dual_stack = true
    subnet_ids                  = aws_subnet.private[*].id
    security_group_ids          = [aws_security_group.application.id]
  }

  environment {
    variables = merge(local.database_environment, {
      COGNITO_CLIENT_ID          = aws_cognito_user_pool_client.api.id
      COGNITO_CLIENT_SECRET      = aws_cognito_user_pool_client.api.client_secret
      COGNITO_USER_POOL_ID       = aws_cognito_user_pool.users.id
      OPERATOR_SUBJECTS          = var.operator_subjects
      JUDGE_CPP_IMAGE            = var.judge_runtime_digest
      JUDGE_RUNTIME              = "cpp17-isolate"
      JUDGE_DISPATCH_FUNCTION    = "${local.name}-judge-bridge"
      JUDGE_ENABLED_RUNTIMES     = length(var.judge_enabled_runtimes) == 0 ? "none" : join(",", var.judge_enabled_runtimes)
      TEST_DATA_BUCKET           = aws_s3_bucket.test_data.id
      AWS_USE_DUALSTACK_ENDPOINT = "true"
    })
  }
}

resource "aws_iam_role_policy" "test_data" {
  name = "test-data"
  role = aws_iam_role.api.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = ["s3:GetObject", "s3:GetObjectVersion", "s3:PutObject", "s3:PutObjectTagging", "s3:PutObjectVersionTagging"]
      Resource = "${aws_s3_bucket.test_data.arn}/test-files/*"
    }]
  })
}

resource "aws_apigatewayv2_api" "api" {
  name          = "${local.name}-api"
  protocol_type = "HTTP"
}

resource "aws_apigatewayv2_integration" "api" {
  api_id                 = aws_apigatewayv2_api.api.id
  integration_type       = "AWS_PROXY"
  integration_method     = "POST"
  integration_uri        = aws_lambda_function.api.invoke_arn
  payload_format_version = "2.0"
  timeout_milliseconds   = 20000
}

resource "aws_apigatewayv2_route" "default" {
  api_id             = aws_apigatewayv2_api.api.id
  route_key          = "$default"
  authorization_type = "NONE"
  target             = "integrations/${aws_apigatewayv2_integration.api.id}"
}

resource "aws_cloudwatch_log_group" "api_gateway" {
  name              = "/aws/apigateway/${local.name}-api"
  retention_in_days = var.log_retention_days
}

resource "aws_apigatewayv2_stage" "default" {
  api_id      = aws_apigatewayv2_api.api.id
  name        = "$default"
  auto_deploy = true
  default_route_settings {
    throttling_burst_limit = var.throttling_burst_limit
    throttling_rate_limit  = var.throttling_rate_limit
  }
  access_log_settings {
    destination_arn = aws_cloudwatch_log_group.api_gateway.arn
    format = jsonencode({
      httpMethod       = "$context.httpMethod"
      integrationError = "$context.integrationErrorMessage"
      ip               = "$context.identity.sourceIp"
      protocol         = "$context.protocol"
      requestId        = "$context.requestId"
      requestTime      = "$context.requestTime"
      responseLength   = "$context.responseLength"
      routeKey         = "$context.routeKey"
      status           = "$context.status"
    })
  }
}

resource "aws_lambda_permission" "api_gateway" {
  statement_id  = "judge-dev-api-ApiGatewayInvokePermission-ygAZJlndnBQw"
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.api.function_name
  principal     = "apigateway.amazonaws.com"
  source_arn    = "${aws_apigatewayv2_api.api.execution_arn}/*"
}
