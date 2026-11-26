# The deploy role is imported, and a mock provider cannot import.
override_resource {
  target = aws_iam_role.github_deploy
  values = { arn = "arn:aws:iam::123456789012:role/judge-dev-github-deploy" }
}
mock_provider "aws" {
  mock_data "aws_caller_identity" { defaults = { account_id = "123456789012" } }
  mock_data "aws_partition" { defaults = { partition = "aws" } }
  mock_resource "aws_iam_role" { defaults = { arn = "arn:aws:iam::123456789012:role/judge-dev-example" } }
}
variables {
  state_bucket               = "example-state"
  github_subject             = "repo:owner@123/project@456:environment:dev"
  github_cost_report_subject = "repo:owner@123/project@456:environment:cost-report"
  api_execution_role_name    = "example-api"
  gateway_ids                = ["example"]
  user_pool_ids              = ["ap-northeast-1_example"]
}
run "cost_report_reads_costs_only" {
  command = plan
  assert {
    condition     = length(jsondecode(aws_iam_role_policy.cost_report.policy).Statement) == 1
    error_message = "The cost report role must have exactly one permission statement."
  }
  assert {
    condition     = jsondecode(aws_iam_role_policy.cost_report.policy).Statement[0].Effect == "Allow"
    error_message = "The cost report statement must be an Allow."
  }
  assert {
    condition     = jsondecode(aws_iam_role_policy.cost_report.policy).Statement[0].Action == ["ce:GetCostAndUsage"]
    error_message = "The cost report role may only read costs."
  }
  assert {
    condition     = aws_iam_role_policy.cost_report.role == "judge-dev-github-cost-report"
    error_message = "The cost policy must attach to the cost report role, not the deploy role."
  }
}
run "cost_report_trust_is_exact" {
  command = plan
  assert {
    condition     = length(jsondecode(aws_iam_role.github_cost_report.assume_role_policy).Statement) == 1
    error_message = "The cost report role must trust exactly one principal."
  }
  assert {
    condition     = jsondecode(aws_iam_role.github_cost_report.assume_role_policy).Statement[0].Action == "sts:AssumeRoleWithWebIdentity"
    error_message = "The cost report role must be assumed through GitHub OIDC."
  }
  assert {
    condition = jsondecode(aws_iam_role.github_cost_report.assume_role_policy).Statement[0].Principal == {
      Federated = "arn:aws:iam::123456789012:oidc-provider/token.actions.githubusercontent.com"
    }
    error_message = "The cost report role must trust only the GitHub OIDC provider of this account."
  }
  assert {
    condition = jsondecode(aws_iam_role.github_cost_report.assume_role_policy).Statement[0].Condition == {
      StringEquals = {
        "token.actions.githubusercontent.com:aud" = "sts.amazonaws.com"
        "token.actions.githubusercontent.com:sub" = "repo:owner@123/project@456:environment:cost-report"
      }
    }
    error_message = "The cost report role must match the exact cost-report environment subject and nothing else."
  }
}
run "cost_report_subject_rejects_dev" {
  command = plan
  variables { github_cost_report_subject = "repo:owner@123/project@456:environment:dev" }
  expect_failures = [var.github_cost_report_subject]
}
run "cost_report_subject_rejects_wildcard" {
  command = plan
  variables { github_cost_report_subject = "repo:owner@123/*:environment:cost-report" }
  expect_failures = [var.github_cost_report_subject]
}
run "cost_report_subject_rejects_branch" {
  command = plan
  variables { github_cost_report_subject = "repo:owner@123/project@456:ref:refs/heads/main" }
  expect_failures = [var.github_cost_report_subject]
}
