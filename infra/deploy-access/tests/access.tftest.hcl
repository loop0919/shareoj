override_resource {
  target = aws_iam_role.github_deploy
  values = { arn = "arn:aws:iam::123456789012:role/judge-dev-github-deploy" }
}
mock_provider "aws" {
  mock_data "aws_caller_identity" { defaults = { account_id = "123456789012" } }
  mock_data "aws_partition" { defaults = { partition = "aws" } }
  mock_resource "aws_iam_role" { defaults = { arn = "arn:aws:iam::123456789012:role/judge-dev-github-deploy" } }
}
variables {
  state_bucket            = "example-state"
  github_subject          = "repo:owner@123/project@456:environment:dev"
  api_execution_role_name = "example-api"
  gateway_ids             = ["example"]
  user_pool_ids           = ["ap-northeast-1_example"]
}
run "deployment_boundary" {
  command = plan
  assert {
    condition = anytrue([for statement in jsondecode(aws_iam_role_policy.deploy.policy).Statement : (
      statement.Effect == "Allow" &&
      statement.Resource == "*" &&
      toset(statement.Action) == toset([
        "logs:CreateLogDelivery",
        "logs:PutResourcePolicy",
        "logs:UpdateLogDelivery",
        "logs:DeleteLogDelivery",
        "logs:CreateLogGroup",
        "logs:DescribeResourcePolicies",
        "logs:GetLogDelivery",
        "logs:ListLogDeliveries",
      ])
    ) if statement.Sid == "ConfigureApiGatewayLogDelivery"])
    error_message = "HTTP API access logging must have the account-level CloudWatch Logs delivery permissions required by AWS."
  }
  assert {
    condition = anytrue([for statement in jsondecode(aws_iam_role_policy.deploy.policy).Statement : (
      contains(statement.Action, "iam:DeleteRole") &&
      contains(statement.Action, "iam:ListInstanceProfilesForRole") &&
      toset(statement.Resource) == toset(local.execution_roles)
    ) if statement.Sid == "ExecutionRoles"])
    error_message = "Terraform must be able to list instance profiles before deleting an application execution role."
  }
  assert {
    condition = anytrue([for statement in jsondecode(aws_iam_role_policy.deploy.policy).Statement : (
      statement.Effect == "Allow" &&
      toset(statement.Action) == toset(["s3:*"]) &&
      statement.Resource == "arn:aws:s3:::judge-dev-test-data-*"
    ) if statement.Sid == "ApplicationTestDataBucket"])
    error_message = "Terraform must manage the private test-data bucket without access to its objects."
  }
  assert {
    condition = anytrue([for statement in jsondecode(aws_iam_role_policy.deploy.policy).Statement : (
      statement.Effect == "Allow" &&
      statement.Resource == "arn:aws:ec2:ap-northeast-1:123456789012:security-group-rule/*" &&
      toset(statement.Action) == toset(["ec2:AuthorizeSecurityGroupIngress", "ec2:AuthorizeSecurityGroupEgress"]) &&
      tomap(statement.Condition.StringEquals) == tomap({ "aws:RequestTag/Project" = "judge", "aws:RequestTag/Environment" = "Dev" })
    ) if statement.Sid == "CreateTaggedSecurityGroupRules"])
    error_message = "New security group rules must be authorized using request tags before resource tags exist."
  }
  assert {
    condition = anytrue([for statement in jsondecode(aws_iam_role_policy.deploy.policy).Statement : (
      contains(statement.Action, "ec2:AuthorizeSecurityGroupIngress") &&
      contains(statement.Action, "ec2:AuthorizeSecurityGroupEgress") &&
      tomap(statement.Condition.StringEquals) == tomap({ "ec2:ResourceTag/Project" = "judge", "ec2:ResourceTag/Environment" = "Dev" })
    ) if statement.Sid == "ManageTaggedNetwork"])
    error_message = "Rule creation must still require the parent security group to have application tags."
  }
  assert {
    condition = anytrue([for statement in jsondecode(aws_iam_role_policy.deploy.policy).Statement : (
      statement.Effect == "Allow" &&
      statement.Resource == "arn:aws:rds:ap-northeast-1:123456789012:db:*" &&
      toset(statement.Action) == toset(["rds:DescribeDBInstances"]) &&
      !can(statement.Condition)
    ) if statement.Sid == "DescribeDatabaseInstances"])
    error_message = "Terraform's dbi-resource-id lookup must allow listing DB instances within the deployment account and region."
  }
  assert {
    condition = alltrue([for statement in jsondecode(aws_iam_role_policy.deploy.policy).Statement : (
      toset(statement.Resource) == toset([
        "arn:aws:rds:ap-northeast-1:123456789012:db:judge-dev-postgres",
        "arn:aws:rds:ap-northeast-1:123456789012:subgrp:judge-dev",
        "arn:aws:rds:ap-northeast-1:123456789012:snapshot:judge-dev-postgres-*"
      ])
    ) if contains(try(statement.Action, []), "rds:ModifyDBInstance") || contains(try(statement.Action, []), "rds:DeleteDBInstance")])
    error_message = "Database write permissions must remain limited to application resources."
  }
  assert {
    condition = anytrue([for statement in jsondecode(aws_iam_role_policy.deploy.policy).Statement : (
      statement.Resource == "arn:aws:ec2:ap-northeast-1:123456789012:vpc/*" &&
      toset(statement.Action) == toset(["ec2:CreateSubnet", "ec2:CreateSecurityGroup", "ec2:CreateEgressOnlyInternetGateway", "ec2:CreateRouteTable"]) &&
      tomap(statement.Condition.StringEquals) == tomap({ "ec2:ResourceTag/Project" = "judge", "ec2:ResourceTag/Environment" = "Dev" })
    ) if statement.Sid == "CreateWithinApplicationVpc"])
    error_message = "Creation must authorize the parent VPC by existing resource tags, separately from new resource request tags."
  }
  assert {
    condition = anytrue([for statement in jsondecode(aws_iam_role_policy.deploy.policy).Statement : (
      statement.Effect == "Deny" && statement.Resource == "*" &&
      toset(statement.NotAction) == toset(["ec2:Describe*", "ec2:Get*"]) &&
      tomap(statement.Condition.StringEquals) == tomap({ "ec2:ResourceTag/Component" = "judge-worker" })
    ) if try(statement.Sid, "") == "ProtectJudgePool"])
    error_message = "The deploy role must not change judge hosts, their network or their hold tags."
  }
  assert {
    condition     = length(aws_iam_role_policy.deploy.policy) <= 10240
    error_message = "Deployment permissions must fit the IAM role inline policy size limit."
  }
  assert {
    condition     = jsondecode(aws_iam_role.github_deploy.assume_role_policy).Statement[0].Condition.StringEquals["token.actions.githubusercontent.com:sub"] == var.github_subject
    error_message = "Trust must match the exact immutable repository and environment subject."
  }
  assert {
    condition     = alltrue([for statement in jsondecode(aws_iam_role_policy.deploy.policy).Statement : !contains(try(statement.Action, []), "iam:*")]) && !contains(local.execution_roles, "arn:aws:iam::123456789012:role/judge-dev-github-deploy")
    error_message = "The deployment role must not grant itself IAM permissions."
  }
}
run "reject_wildcard_subject" {
  command = plan
  variables { github_subject = "repo:owner/*:environment:dev" }
  expect_failures = [var.github_subject]
}
