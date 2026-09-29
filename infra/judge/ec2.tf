# EC2 judge pool (ADR 0012): one always-on primary host and burst hosts that the bridge
# starts for contests. Burst hosts power themselves off when idle past their hold tags.
# The pool has its own IPv6-only VPC; it has no route to the application VPC or database.
locals {
  pool_hosts = merge(
    { "${local.name}-judge-primary" = { role = "primary", index = 0 } },
    { for i in range(var.burst_worker_count) : "${local.name}-judge-burst-${i + 1}" => { role = "burst", index = i + 1 } },
  )
  # The deploy-access role must not modify these resources; see infra/deploy-access.
  pool_tags = { Component = "judge-worker" }
  pool_environment = { for name, host in local.pool_hosts : name => join("\n", [
    "AWS_DEFAULT_REGION=${var.aws_region}",
    "AWS_EC2_METADATA_SERVICE_ENDPOINT_MODE=IPv6",
    "JUDGE_POOL_ROLE=${host.role}",
    "JUDGE_JOB_BUCKET=${aws_s3_bucket.jobs.id}",
    "JUDGE_TEST_DATA_BUCKET=${var.test_data_bucket == null ? "" : var.test_data_bucket.id}",
    "JUDGE_REQUEST_QUEUE_URL=${aws_sqs_queue.queue["requests"].url}",
    "JUDGE_RESULT_QUEUE_URL=${aws_sqs_queue.queue["results"].url}",
    "JUDGE_RUNTIME_DIGEST=${var.runtime_digest}",
  ]) }
  # AmazonSSMManagedInstanceCore without ssm:GetParameter*: instance credentials are what
  # a sandbox escape would obtain.
  ssm_agent_statements = [
    {
      Effect = "Allow", Resource = "*", Action = [
        "ssm:UpdateInstanceInformation", "ssm:ListAssociations", "ssm:ListInstanceAssociations", "ssm:DescribeAssociation",
        "ssm:GetDocument", "ssm:DescribeDocument", "ssm:UpdateAssociationStatus", "ssm:UpdateInstanceAssociationStatus",
        "ssmmessages:CreateControlChannel", "ssmmessages:CreateDataChannel", "ssmmessages:OpenControlChannel", "ssmmessages:OpenDataChannel",
        "ec2messages:AcknowledgeMessage", "ec2messages:DeleteMessage", "ec2messages:FailMessage", "ec2messages:GetEndpoint",
        "ec2messages:GetMessages", "ec2messages:SendReply"
      ]
    }
  ]
}

resource "aws_vpc" "pool" {
  # AWS requires an IPv4 block; the subnets are IPv6-only and never use it.
  cidr_block                       = "10.43.0.0/16"
  assign_generated_ipv6_cidr_block = true
  enable_dns_support               = true
  enable_dns_hostnames             = true
  tags                             = merge(local.pool_tags, { Name = "${local.name}-judge-pool" })
}
resource "aws_subnet" "pool" {
  for_each                                       = { for i, zone in var.worker_availability_zones : zone => i }
  vpc_id                                         = aws_vpc.pool.id
  availability_zone                              = each.key
  ipv6_native                                    = true
  ipv6_cidr_block                                = cidrsubnet(aws_vpc.pool.ipv6_cidr_block, 8, each.value)
  assign_ipv6_address_on_creation                = true
  enable_resource_name_dns_aaaa_record_on_launch = true
  private_dns_hostname_type_on_launch            = "resource-name"
  tags                                           = merge(local.pool_tags, { Name = "${local.name}-judge-${each.key}" })
}
resource "aws_egress_only_internet_gateway" "pool" {
  vpc_id = aws_vpc.pool.id
  tags   = merge(local.pool_tags, { Name = "${local.name}-judge-pool" })
}
resource "aws_route_table" "pool" {
  vpc_id = aws_vpc.pool.id
  route {
    ipv6_cidr_block        = "::/0"
    egress_only_gateway_id = aws_egress_only_internet_gateway.pool.id
  }
  tags = merge(local.pool_tags, { Name = "${local.name}-judge-pool" })
}
resource "aws_route_table_association" "pool" {
  for_each       = aws_subnet.pool
  subnet_id      = each.value.id
  route_table_id = aws_route_table.pool.id
}
# No ingress: operators use SSM. Amazon DNS, NTP and IMDS are not filtered by security groups.
resource "aws_security_group" "pool" {
  name        = "${local.name}-judge-pool"
  description = "Judge hosts: outbound HTTPS to dual-stack AWS endpoints and Ubuntu archives only"
  vpc_id      = aws_vpc.pool.id
  tags        = merge(local.pool_tags, { Name = "${local.name}-judge-pool" })
}
resource "aws_vpc_security_group_egress_rule" "pool_https" {
  security_group_id = aws_security_group.pool.id
  cidr_ipv6         = "::/0"
  ip_protocol       = "tcp"
  from_port         = 443
  to_port           = 443
  tags              = local.pool_tags
}

resource "aws_iam_role" "pool" {
  name = "${local.name}-judge-pool"
  assume_role_policy = jsonencode({
    Version   = "2012-10-17"
    Statement = [{ Effect = "Allow", Principal = { Service = "ec2.amazonaws.com" }, Action = "sts:AssumeRole" }]
  })
}
resource "aws_iam_role_policy" "pool" {
  role   = aws_iam_role.pool.id
  policy = jsonencode({ Version = "2012-10-17", Statement = concat(local.worker_statements, local.observability_statements, local.ssm_agent_statements) })
}
resource "aws_iam_instance_profile" "pool" {
  name = "${local.name}-judge-pool"
  role = aws_iam_role.pool.name
}

data "aws_ssm_parameter" "ubuntu" {
  name = "/aws/service/canonical/ubuntu/server/24.04/stable/current/amd64/hvm/ebs-gp3/ami-id"
}
resource "aws_instance" "pool" {
  for_each      = local.pool_hosts
  ami           = data.aws_ssm_parameter.ubuntu.insecure_value
  instance_type = "t3a.small"
  subnet_id     = aws_subnet.pool[var.worker_availability_zones[each.value.index % length(var.worker_availability_zones)]].id
  # Every host must share one runtime digest, so hosts are installed together by rollout.py.
  vpc_security_group_ids               = [aws_security_group.pool.id]
  iam_instance_profile                 = aws_iam_instance_profile.pool.name
  ipv6_address_count                   = 1
  associate_public_ip_address          = false
  instance_initiated_shutdown_behavior = "stop"
  credit_specification {
    # Standard mode would throttle a freshly started host to 20% of a vCPU.
    cpu_credits = "unlimited"
  }
  metadata_options {
    http_endpoint               = "enabled"
    http_tokens                 = "required"
    http_put_response_hop_limit = 1
    http_protocol_ipv6          = "enabled"
    instance_metadata_tags      = "enabled"
  }
  root_block_device {
    volume_type           = "gp3"
    volume_size           = var.worker_volume_size
    encrypted             = true
    delete_on_termination = true
    tags                  = merge(local.pool_tags, { Name = each.key })
  }
  user_data = templatefile("${path.module}/user-data-ec2.sh.tftpl", { region = var.aws_region, worker_environment = local.pool_environment[each.key] })
  tags      = merge(local.pool_tags, { Name = each.key, JudgeRole = each.value.role, JudgePool = local.name })
  lifecycle {
    # A new AMI or bootstrap must not replace installed hosts; OS changes go through rollout.
    ignore_changes = [ami, user_data]
  }
}
