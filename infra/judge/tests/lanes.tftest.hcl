mock_provider "aws" {
  mock_resource "aws_sns_topic" { defaults = { arn = "arn:aws:sns:ap-northeast-1:123456789012:judge-test" } }
  mock_resource "aws_s3_bucket" { defaults = { arn = "arn:aws:s3:::judge-test-jobs" } }
  mock_resource "aws_s3_object" { defaults = { arn = "arn:aws:s3:::judge-test-jobs/releases/test", version_id = "test-version" } }
  mock_resource "aws_sqs_queue" { defaults = { arn = "arn:aws:sqs:ap-northeast-1:123456789012:judge-test", url = "https://sqs.ap-northeast-1.amazonaws.com/123456789012/judge-test" } }
  mock_resource "aws_iam_role" { defaults = { arn = "arn:aws:iam::123456789012:role/judge-test" } }
  mock_resource "aws_lambda_function" { defaults = { arn = "arn:aws:lambda:ap-northeast-1:123456789012:function:judge-test" } }
  mock_resource "aws_cloudwatch_log_group" { defaults = { arn = "arn:aws:logs:ap-northeast-1:123456789012:log-group:judge-test" } }
  mock_resource "aws_cloudwatch_event_rule" { defaults = { arn = "arn:aws:events:ap-northeast-1:123456789012:rule/judge-test" } }
  mock_resource "aws_vpc" { defaults = { ipv6_cidr_block = "2001:db8:1234:5600::/56" } }
  mock_resource "aws_iam_instance_profile" { defaults = { arn = "arn:aws:iam::123456789012:instance-profile/judge-test" } }
  mock_data "aws_ssm_parameter" { defaults = { insecure_value = "ami-0123456789abcdef0", value = "ami-0123456789abcdef0" } }
}
variables {
  request_lane               = "a"
  discord_webhook_secret_arn = ""
  alerts_enabled             = false
  enabled                    = false
  ssh_public_key             = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJbU10sbvSiPykk/v/mzxDSkNPF1hvszNuRt/RLGKd5L"
  admin_ipv6_cidr            = "2001:db8::1/128"
  runtime_digest             = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
  enabled_runtimes           = ["cpp17", "python314"]
  bridge_package_path        = "tests/package.txt"
  notify_package_path        = "tests/package.txt"
  database = {
    host              = "db.example.rds.amazonaws.com"
    name              = "openoj"
    secret_arn        = "arn:aws:secretsmanager:ap-northeast-1:123456789012:secret:db-test"
    subnet_ids        = ["subnet-12345678", "subnet-87654321"]
    security_group_id = "sg-12345678"
  }
  test_data_bucket = {
    id  = "judge-test-data"
    arn = "arn:aws:s3:::judge-test-data"
  }
}
# Kept apart from judge.tftest.hcl: runs in one file share state after an apply run,
# and overriding a queue URL has no effect on resources already in that state.
run "request_lanes" {
  command = plan
  assert {
    condition = (
      aws_sqs_queue.queue["requests"].name == "judge-dev-judge-requests" &&
      endswith(aws_sqs_queue.queue["requests-b"].name, "-judge-requests-b") &&
      aws_sqs_queue.queue["requests-b"].visibility_timeout_seconds == 2100
    )
    error_message = "Lane a keeps its name and lane b is a second request queue with the same visibility timeout."
  }
}

run "lane_b_selects_queue" {
  command = plan
  variables { request_lane = "b" }
  override_resource {
    override_during = plan
    target          = aws_sqs_queue.queue["requests"]
    values          = { url = "https://sqs.ap-northeast-1.amazonaws.com/123456789012/lane-a" }
  }
  override_resource {
    override_during = plan
    target          = aws_sqs_queue.queue["requests-b"]
    values          = { url = "https://sqs.ap-northeast-1.amazonaws.com/123456789012/lane-b" }
  }
  override_data {
    target = data.aws_lambda_function.bridge_live[0]
    values = {
      environment = [{ variables = { JUDGE_REQUEST_QUEUE_URL = "https://sqs.ap-northeast-1.amazonaws.com/123456789012/lane-b" } }]
    }
  }
  assert {
    condition     = aws_lambda_function.bridge.environment[0].variables.JUDGE_REQUEST_QUEUE_URL == "https://sqs.ap-northeast-1.amazonaws.com/123456789012/lane-b"
    error_message = "The bridge must send to the selected lane."
  }
}

# SQS checks the dead letter queue's allow policy when a source queue names it, so the policy must list lane b
# before lane b is created. Deriving the ARNs from the DLQ's own ARN keeps the policy off the queues' ARNs:
# with only the DLQ ARNs known, the policy is complete.
run "dead_letter_allows_both_lanes_before_they_exist" {
  command = plan
  override_resource {
    override_during = plan
    target          = aws_sqs_queue.dead["requests"]
    values          = { arn = "arn:aws:sqs:ap-northeast-1:123456789012:judge-dev-judge-requests-dead" }
  }
  override_resource {
    override_during = plan
    target          = aws_sqs_queue.dead["results"]
    values          = { arn = "arn:aws:sqs:ap-northeast-1:123456789012:judge-dev-judge-results-dead" }
  }
  assert {
    condition = jsondecode(aws_sqs_queue_redrive_allow_policy.dead["requests"].redrive_allow_policy).sourceQueueArns == [
      "arn:aws:sqs:ap-northeast-1:123456789012:judge-dev-judge-requests",
      "arn:aws:sqs:ap-northeast-1:123456789012:judge-dev-judge-requests-b",
    ]
    error_message = "The request DLQ must allow lane a and lane b by ARNs that do not wait for the queues."
  }
  assert {
    condition     = jsondecode(aws_sqs_queue_redrive_allow_policy.dead["results"].redrive_allow_policy).sourceQueueArns == ["arn:aws:sqs:ap-northeast-1:123456789012:judge-dev-judge-results"]
    error_message = "The result DLQ keeps allowing only the result queue."
  }
}

run "invalid_lane" {
  command = plan
  variables { request_lane = "c" }
  expect_failures = [var.request_lane]
}

run "stale_lane_fails" {
  command = plan
  override_resource {
    override_during = plan
    target          = aws_sqs_queue.queue["requests"]
    values          = { url = "https://sqs.ap-northeast-1.amazonaws.com/123456789012/lane-a" }
  }
  override_resource {
    override_during = plan
    target          = aws_sqs_queue.queue["requests-b"]
    values          = { url = "https://sqs.ap-northeast-1.amazonaws.com/123456789012/lane-b" }
  }
  override_data {
    target = data.aws_lambda_function.bridge_live[0]
    values = {
      environment = [{ variables = { JUDGE_REQUEST_QUEUE_URL = "https://sqs.ap-northeast-1.amazonaws.com/123456789012/lane-b" } }]
    }
  }
  expect_failures = [aws_lambda_function.bridge]
}

run "first_build_without_live_bridge" {
  command = plan
  variables { check_live_request_lane = false }
}

run "live_lane_matches" {
  command = plan
  override_resource {
    override_during = plan
    target          = aws_sqs_queue.queue["requests"]
    values          = { url = "https://sqs.ap-northeast-1.amazonaws.com/123456789012/lane-a" }
  }
  override_data {
    target = data.aws_lambda_function.bridge_live[0]
    values = {
      environment = [{ variables = { JUDGE_REQUEST_QUEUE_URL = "https://sqs.ap-northeast-1.amazonaws.com/123456789012/lane-a" } }]
    }
  }
}

run "lane_b_while_live_is_a" {
  command = plan
  variables { request_lane = "b" }
  override_resource {
    override_during = plan
    target          = aws_sqs_queue.queue["requests"]
    values          = { url = "https://sqs.ap-northeast-1.amazonaws.com/123456789012/lane-a" }
  }
  override_resource {
    override_during = plan
    target          = aws_sqs_queue.queue["requests-b"]
    values          = { url = "https://sqs.ap-northeast-1.amazonaws.com/123456789012/lane-b" }
  }
  override_data {
    target = data.aws_lambda_function.bridge_live[0]
    values = {
      environment = [{ variables = { JUDGE_REQUEST_QUEUE_URL = "https://sqs.ap-northeast-1.amazonaws.com/123456789012/lane-a" } }]
    }
  }
  expect_failures = [aws_lambda_function.bridge]
}
