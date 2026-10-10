mock_provider "aws" {
  mock_resource "aws_s3_bucket" { defaults = { arn = "arn:aws:s3:::example-web-artifacts" } }
  mock_resource "aws_cloudwatch_log_group" { defaults = { arn = "arn:aws:logs:ap-northeast-1:123456789012:log-group:example" } }
  mock_resource "aws_iam_role" { defaults = { arn = "arn:aws:iam::123456789012:role/example" } }
  mock_resource "aws_lambda_function" { defaults = { invoke_arn = "arn:aws:apigateway:ap-northeast-1:lambda:path/2015-03-31/functions/arn:aws:lambda:ap-northeast-1:123456789012:function:example/invocations" } }
  mock_resource "aws_apigatewayv2_api" { defaults = { execution_arn = "arn:aws:execute-api:ap-northeast-1:123456789012:example", api_endpoint = "https://example.execute-api.ap-northeast-1.amazonaws.com" } }
}
variables {
  api_endpoint    = "https://api.example.com"
  public_site_url = ""
}
run "frontend_contract" {
  command = apply
  assert {
    condition = (
      length(aws_lambda_function.web.vpc_config) == 0 &&
      aws_lambda_function.web.runtime == "nodejs22.x" &&
      aws_lambda_function.web.handler == "lambda/index.handler" &&
      aws_lambda_function.web.architectures == tolist(["arm64"]) &&
      aws_lambda_function.web.memory_size == 512 &&
      aws_lambda_function.web.reserved_concurrent_executions == 200 &&
      aws_lambda_function.web.environment[0].variables["NUXT_API_BASE_URL"] == var.api_endpoint &&
      aws_lambda_function.web.environment[0].variables["NUXT_PUBLIC_SITE_URL"] == aws_apigatewayv2_api.web.api_endpoint &&
      aws_lambda_function.web.s3_object_version == aws_s3_object.web_package.version_id
    )
    error_message = "Deploy the exact Lambda bundle and use the public HTTPS origin for canonical URLs."
  }
  assert {
    condition = (
      aws_apigatewayv2_route.default.target == "integrations/${aws_apigatewayv2_integration.web.id}" &&
      aws_apigatewayv2_integration.web.payload_format_version == "2.0" &&
      aws_apigatewayv2_stage.default.default_route_settings[0].throttling_burst_limit == 500 &&
      aws_apigatewayv2_stage.default.default_route_settings[0].throttling_rate_limit == 200 &&
      aws_lambda_permission.gateway.source_arn == "${aws_apigatewayv2_api.web.execution_arn}/*" &&
      aws_cloudwatch_log_group.lambda.retention_in_days == 14 &&
      aws_s3_bucket_public_access_block.artifacts.block_public_policy &&
      aws_s3_bucket_versioning.artifacts.versioning_configuration[0].status == "Enabled"
    )
    error_message = "Keep public routing, scoped invocation, log retention, and private versioned artifacts."
  }
  assert {
    condition     = jsondecode(aws_apigatewayv2_stage.default.access_log_settings[0].format).ip == "$context.identity.sourceIp"
    error_message = "Record the viewer IP in access logs so abuse can be traced."
  }
}
run "configured_public_site_url" {
  command = plan
  variables { public_site_url = "https://judge.example.com" }
  assert {
    condition     = aws_lambda_function.web.environment[0].variables["NUXT_PUBLIC_SITE_URL"] == "https://judge.example.com" && output.site_url == "https://judge.example.com"
    error_message = "Use the configured public origin for frontend URLs, including OAuth callbacks."
  }
}
run "reject_non_https_api" {
  command = plan
  variables { api_endpoint = "http://api.example.com" }
  expect_failures = [var.api_endpoint]
}
