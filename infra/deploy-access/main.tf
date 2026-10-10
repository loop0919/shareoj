data "aws_caller_identity" "current" {}
data "aws_partition" "current" {}
locals {
  account = data.aws_caller_identity.current.account_id
  arn     = "arn:${data.aws_partition.current.partition}"
  execution_roles = [
    "${local.arn}:iam::${local.account}:role/${var.api_execution_role_name}",
    "${local.arn}:iam::${local.account}:role/${local.name}-web",
  ]
}
# Adopt the role created in the console; only this bootstrap stack manages its trust.
resource "aws_iam_role" "github_deploy" {
  name = "${local.name}-github-deploy"
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Federated = "${local.arn}:iam::${local.account}:oidc-provider/token.actions.githubusercontent.com" }
      Action    = "sts:AssumeRoleWithWebIdentity"
      Condition = { StringEquals = {
        "token.actions.githubusercontent.com:aud" = "sts.amazonaws.com"
        "token.actions.githubusercontent.com:sub" = var.github_subject
      } }
    }]
  })
  lifecycle { prevent_destroy = true }
}
resource "aws_iam_role_policy" "deploy" {
  name = "${local.name}-deploy"
  role = aws_iam_role.github_deploy.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid      = "StateBucket"
        Effect   = "Allow"
        Action   = ["s3:ListBucket", "s3:GetBucketLocation"]
        Resource = "${local.arn}:s3:::${var.state_bucket}"
      },
      {
        Sid      = "ApplicationState"
        Effect   = "Allow"
        Action   = ["s3:GetObject", "s3:PutObject"]
        Resource = [for app in ["api", "frontend"] : "${local.arn}:s3:::${var.state_bucket}/${var.project_name}/${var.environment}/${app}.tfstate"]
      },
      {
        Sid      = "ApplicationLocks"
        Effect   = "Allow"
        Action   = ["s3:GetObject", "s3:PutObject", "s3:DeleteObject"]
        Resource = [for app in ["api", "frontend"] : "${local.arn}:s3:::${var.state_bucket}/${var.project_name}/${var.environment}/${app}.tfstate.tflock"]
      },
      {
        Sid      = "ApplicationArtifacts"
        Effect   = "Allow"
        Action   = ["s3:*"]
        Resource = flatten([for prefix in ["${local.name}-artifacts-", "${local.name}-web-artifacts-"] : ["${local.arn}:s3:::${prefix}*", "${local.arn}:s3:::${prefix}*/*"]])
      },
      {
        Sid      = "ApplicationTestDataBucket"
        Effect   = "Allow"
        Action   = ["s3:*"]
        Resource = "${local.arn}:s3:::${local.name}-test-data-*"
      },
      {
        Sid      = "ApplicationFunctions"
        Effect   = "Allow"
        Action   = ["lambda:*"]
        Resource = [for app in ["api", "web"] : "${local.arn}:lambda:${var.aws_region}:${local.account}:function:${local.name}-${app}*"]
      },
      {
        Sid      = "ApplicationLogs"
        Effect   = "Allow"
        Action   = ["logs:*"]
        Resource = [for service in ["lambda", "apigateway"] : "${local.arn}:logs:${var.aws_region}:${local.account}:log-group:/aws/${service}/${local.name}-*"]
      },
      {
        Sid      = "DescribeLogGroups"
        Effect   = "Allow"
        Action   = ["logs:DescribeLogGroups"]
        Resource = "*"
      },
      {
        # HTTP API access logging uses account-level CloudWatch Logs delivery APIs.
        Sid    = "ConfigureApiGatewayLogDelivery"
        Effect = "Allow"
        Action = [
          "logs:CreateLogDelivery",
          "logs:PutResourcePolicy",
          "logs:UpdateLogDelivery",
          "logs:DeleteLogDelivery",
          "logs:CreateLogGroup",
          "logs:DescribeResourcePolicies",
          "logs:GetLogDelivery",
          "logs:ListLogDeliveries",
        ]
        Resource = "*"
      },
      {
        Sid      = "ExecutionRoles"
        Effect   = "Allow"
        Action   = ["iam:GetRole", "iam:CreateRole", "iam:DeleteRole", "iam:ListInstanceProfilesForRole", "iam:UpdateRole", "iam:UpdateAssumeRolePolicy", "iam:TagRole", "iam:UntagRole", "iam:ListRolePolicies", "iam:ListAttachedRolePolicies", "iam:GetRolePolicy", "iam:PutRolePolicy", "iam:DeleteRolePolicy"]
        Resource = local.execution_roles
      },
      {
        Sid       = "PassExecutionRoleToLambda"
        Effect    = "Allow"
        Action    = ["iam:PassRole"]
        Resource  = local.execution_roles
        Condition = { StringEquals = { "iam:PassedToService" = "lambda.amazonaws.com" } }
      },
      {
        Sid      = "ApplicationGateways"
        Effect   = "Allow"
        Action   = ["apigateway:GET", "apigateway:POST", "apigateway:PUT", "apigateway:PATCH", "apigateway:DELETE"]
        Resource = flatten([for id in var.gateway_ids : ["${local.arn}:apigateway:${var.aws_region}::/apis/${id}", "${local.arn}:apigateway:${var.aws_region}::/apis/${id}/*", "${local.arn}:apigateway:${var.aws_region}::/tags/${local.arn}:apigateway:${var.aws_region}::/apis/${id}"]])
      },
      {
        Sid      = "ApplicationUserPools"
        Effect   = "Allow"
        Action   = ["cognito-idp:GetUserPoolMfaConfig", "cognito-idp:SetUserPoolMfaConfig", "cognito-idp:DescribeUserPool", "cognito-idp:UpdateUserPool", "cognito-idp:DescribeUserPoolClient", "cognito-idp:CreateUserPoolClient", "cognito-idp:UpdateUserPoolClient", "cognito-idp:DeleteUserPoolClient", "cognito-idp:CreateIdentityProvider", "cognito-idp:DescribeIdentityProvider", "cognito-idp:UpdateIdentityProvider", "cognito-idp:DeleteIdentityProvider", "cognito-idp:CreateUserPoolDomain", "cognito-idp:UpdateUserPoolDomain", "cognito-idp:DeleteUserPoolDomain", "cognito-idp:ListTagsForResource", "cognito-idp:TagResource", "cognito-idp:UntagResource"]
        Resource = [for id in var.user_pool_ids : "${local.arn}:cognito-idp:${var.aws_region}:${local.account}:userpool/${id}"]
      }
      ,
      {
        Sid      = "DescribeGoogleDomain"
        Effect   = "Allow"
        Action   = ["cognito-idp:DescribeUserPoolDomain"]
        Resource = "*"
      },
      {
        Sid      = "ApplicationDatabase"
        Effect   = "Allow"
        Action   = ["rds:CreateDBInstance", "rds:ModifyDBInstance", "rds:DeleteDBInstance", "rds:CreateDBSubnetGroup", "rds:ModifyDBSubnetGroup", "rds:DeleteDBSubnetGroup", "rds:DescribeDBSubnetGroups", "rds:AddTagsToResource", "rds:RemoveTagsFromResource", "rds:ListTagsForResource", "rds:CreateDBSnapshot"]
        Resource = ["${local.arn}:rds:${var.aws_region}:${local.account}:db:${local.name}-postgres", "${local.arn}:rds:${var.aws_region}:${local.account}:subgrp:${local.name}", "${local.arn}:rds:${var.aws_region}:${local.account}:snapshot:${local.name}-postgres-*"]
      },
      {
        # Terraform filters by dbi-resource-id, which requires listing DB instances.
        Sid      = "DescribeDatabaseInstances"
        Effect   = "Allow"
        Action   = ["rds:DescribeDBInstances"]
        Resource = "${local.arn}:rds:${var.aws_region}:${local.account}:db:*"
      },
      {
        Sid      = "DatabaseLogs"
        Effect   = "Allow"
        Action   = ["logs:*"]
        Resource = "${local.arn}:logs:${var.aws_region}:${local.account}:log-group:/aws/rds/instance/${local.name}-postgres/*"
      },
      {
        Sid      = "DescribeNetwork"
        Effect   = "Allow"
        Action   = ["ec2:DescribeAvailabilityZones", "ec2:DescribeVpcs", "ec2:DescribeVpcAttribute", "ec2:DescribeSubnets", "ec2:DescribeSecurityGroups", "ec2:DescribeSecurityGroupRules", "ec2:DescribeEgressOnlyInternetGateways", "ec2:GetSecurityGroupsForVpc", "ec2:DescribeRouteTables", "ec2:DescribeNetworkInterfaces", "ec2:DescribeTags"]
        Resource = "*"
      },
      {
        Sid       = "CreateTaggedNetwork"
        Effect    = "Allow"
        Action    = ["ec2:CreateVpc", "ec2:CreateSubnet", "ec2:CreateSecurityGroup", "ec2:CreateEgressOnlyInternetGateway", "ec2:CreateRouteTable"]
        Resource  = "*"
        Condition = { StringEquals = { "aws:RequestTag/Project" = var.project_name, "aws:RequestTag/Environment" = title(var.environment) } }
      },
      {
        # EC2 separately authorizes the existing VPC, where RequestTag is absent.
        Sid       = "CreateWithinApplicationVpc"
        Effect    = "Allow"
        Action    = ["ec2:CreateSubnet", "ec2:CreateSecurityGroup", "ec2:CreateEgressOnlyInternetGateway", "ec2:CreateRouteTable"]
        Resource  = "${local.arn}:ec2:${var.aws_region}:${local.account}:vpc/*"
        Condition = { StringEquals = { "ec2:ResourceTag/Project" = var.project_name, "ec2:ResourceTag/Environment" = title(var.environment) } }
      },
      {
        # New rules have request tags; the parent group is checked by ManageTaggedNetwork.
        Sid       = "CreateTaggedSecurityGroupRules"
        Effect    = "Allow"
        Action    = ["ec2:AuthorizeSecurityGroupIngress", "ec2:AuthorizeSecurityGroupEgress"]
        Resource  = "${local.arn}:ec2:${var.aws_region}:${local.account}:security-group-rule/*"
        Condition = { StringEquals = { "aws:RequestTag/Project" = var.project_name, "aws:RequestTag/Environment" = title(var.environment) } }
      },
      {
        Sid       = "TagNewNetwork"
        Effect    = "Allow"
        Action    = ["ec2:CreateTags"]
        Resource  = "${local.arn}:ec2:${var.aws_region}:${local.account}:*/*"
        Condition = { StringEquals = { "aws:RequestTag/Project" = var.project_name, "aws:RequestTag/Environment" = title(var.environment) } }
      },
      {
        Sid       = "ManageTaggedNetwork"
        Effect    = "Allow"
        Action    = ["ec2:ModifyVpcAttribute", "ec2:DeleteVpc", "ec2:ModifySubnetAttribute", "ec2:DeleteSubnet", "ec2:DeleteSecurityGroup", "ec2:AuthorizeSecurityGroupIngress", "ec2:AuthorizeSecurityGroupEgress", "ec2:RevokeSecurityGroupIngress", "ec2:RevokeSecurityGroupEgress", "ec2:ModifySecurityGroupRules", "ec2:AssociateVpcCidrBlock", "ec2:DisassociateVpcCidrBlock", "ec2:DeleteEgressOnlyInternetGateway", "ec2:CreateRoute", "ec2:DeleteRoute", "ec2:ReplaceRoute", "ec2:AssociateRouteTable", "ec2:DisassociateRouteTable", "ec2:ReplaceRouteTableAssociation", "ec2:DeleteRouteTable", "ec2:CreateTags", "ec2:DeleteTags"]
        Resource  = "${local.arn}:ec2:${var.aws_region}:${local.account}:*/*"
        Condition = { StringEquals = { "ec2:ResourceTag/Project" = var.project_name, "ec2:ResourceTag/Environment" = title(var.environment) } }
      },
      {
        Sid      = "RDSManagedPassword"
        Effect   = "Allow"
        Action   = ["secretsmanager:CreateSecret", "secretsmanager:TagResource", "secretsmanager:DescribeSecret", "secretsmanager:RotateSecret"]
        Resource = "${local.arn}:secretsmanager:${var.aws_region}:${local.account}:secret:rds!db-*"
      },
      {
        Sid      = "DescribeDatabaseKey"
        Effect   = "Allow"
        Action   = ["kms:DescribeKey"]
        Resource = "${local.arn}:kms:${var.aws_region}:${local.account}:key/*"
      },
      {
        Sid       = "RDSServiceLinkedRole"
        Effect    = "Allow"
        Action    = ["iam:CreateServiceLinkedRole"]
        Resource  = "${local.arn}:iam::${local.account}:role/aws-service-role/rds.amazonaws.com/AWSServiceRoleForRDS"
        Condition = { StringEquals = { "iam:AWSServiceName" = "rds.amazonaws.com" } }
      },
      {
        # Judge hosts (infra/judge) carry the application tags above; only operators may change them.
        Sid       = "ProtectJudgePool"
        Effect    = "Deny"
        NotAction = ["ec2:Describe*", "ec2:Get*"]
        Resource  = "*"
        Condition = { StringEquals = { "ec2:ResourceTag/Component" = "judge-worker" } }
      },
      {
        # Deployments only touch the application region; IAM and STS are global services.
        Sid       = "DenyOtherRegions"
        Effect    = "Deny"
        NotAction = ["iam:*", "sts:*"]
        Resource  = "*"
        Condition = { StringNotEquals = { "aws:RequestedRegion" = var.aws_region } }
      }
    ]
  })
}
output "deploy_role_arn" { value = aws_iam_role.github_deploy.arn }
