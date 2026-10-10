locals {
  lambda_package_path = coalesce(var.lambda_package_path, "${path.module}/../../web/.build/web.zip")
}
resource "aws_cloudwatch_log_group" "lambda" {
  name              = "/aws/lambda/${local.name}-web"
  retention_in_days = 14
}
resource "aws_cloudwatch_log_group" "gateway" {
  name              = "/aws/apigateway/${local.name}-web"
  retention_in_days = 14
}
resource "aws_iam_role" "web" {
  name = "${local.name}-web"
  assume_role_policy = jsonencode({
    Version   = "2012-10-17"
    Statement = [{ Effect = "Allow", Principal = { Service = "lambda.amazonaws.com" }, Action = "sts:AssumeRole" }]
  })
}
resource "aws_iam_role_policy" "logs" {
  name = "cloudwatch-logs"
  role = aws_iam_role.web.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = ["logs:CreateLogStream", "logs:PutLogEvents"]
      Resource = "${aws_cloudwatch_log_group.lambda.arn}:*"
    }]
  })
}
resource "aws_lambda_function" "web" {
  function_name                  = "${local.name}-web"
  description                    = "OpenOJ Nuxt SSR frontend"
  runtime                        = "nodejs22.x"
  handler                        = "lambda/index.handler"
  architectures                  = ["arm64"]
  memory_size                    = 512
  timeout                        = 25
  role                           = aws_iam_role.web.arn
  s3_bucket                      = aws_s3_object.web_package.bucket
  s3_key                         = aws_s3_object.web_package.key
  s3_object_version              = aws_s3_object.web_package.version_id
  source_code_hash               = filebase64sha256(local.lambda_package_path)
  reserved_concurrent_executions = 200
  environment {
    variables = {
      NODE_ENV                   = "production"
      NUXT_API_BASE_URL          = var.api_endpoint
      NUXT_PUBLIC_SITE_URL       = var.public_site_url != "" ? var.public_site_url : aws_apigatewayv2_api.web.api_endpoint
      NUXT_COGNITO_DOMAIN        = var.cognito_domain
      NUXT_COGNITO_CLIENT_ID     = var.cognito_client_id
      NUXT_COGNITO_CLIENT_SECRET = var.cognito_client_secret
    }
  }
  depends_on = [aws_iam_role_policy.logs]
}
resource "aws_apigatewayv2_api" "web" {
  name          = "${local.name}-web"
  protocol_type = "HTTP"
}
resource "aws_apigatewayv2_integration" "web" {
  api_id                 = aws_apigatewayv2_api.web.id
  integration_type       = "AWS_PROXY"
  integration_method     = "POST"
  integration_uri        = aws_lambda_function.web.invoke_arn
  payload_format_version = "2.0"
  timeout_milliseconds   = 29000
}
resource "aws_apigatewayv2_route" "default" {
  api_id    = aws_apigatewayv2_api.web.id
  route_key = "$default"
  target    = "integrations/${aws_apigatewayv2_integration.web.id}"
}
resource "aws_apigatewayv2_stage" "default" {
  api_id      = aws_apigatewayv2_api.web.id
  name        = "$default"
  auto_deploy = true
  default_route_settings {
    throttling_burst_limit = 500
    throttling_rate_limit  = 200
  }
  access_log_settings {
    destination_arn = aws_cloudwatch_log_group.gateway.arn
    format          = jsonencode({ requestId = "$context.requestId", ip = "$context.identity.sourceIp", httpMethod = "$context.httpMethod", status = "$context.status", responseLength = "$context.responseLength" })
  }
}
resource "aws_lambda_permission" "gateway" {
  statement_id  = "AllowFrontendGateway"
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.web.function_name
  principal     = "apigateway.amazonaws.com"
  source_arn    = "${aws_apigatewayv2_api.web.execution_arn}/*"
}
