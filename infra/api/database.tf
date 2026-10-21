data "aws_availability_zones" "available" { state = "available" }

resource "aws_vpc" "application" {
  cidr_block                       = "10.42.0.0/16"
  assign_generated_ipv6_cidr_block = true
  enable_dns_support               = true
  enable_dns_hostnames             = true
  tags                             = { Name = local.name }
}
resource "aws_subnet" "private" {
  count             = 2
  vpc_id            = aws_vpc.application.id
  cidr_block        = cidrsubnet(aws_vpc.application.cidr_block, 8, count.index)
  ipv6_cidr_block   = cidrsubnet(aws_vpc.application.ipv6_cidr_block, 8, count.index)
  availability_zone = data.aws_availability_zones.available.names[count.index]
  tags              = { Name = "${local.name}-private-${count.index}" }
}
# Cognito and Secrets Manager support IPv6: no hourly NAT or public IPv4 cost.
# This gateway permits outbound connections and their replies only.
resource "aws_egress_only_internet_gateway" "application" {
  vpc_id = aws_vpc.application.id
  tags   = { Name = local.name }
}
resource "aws_route_table" "private" {
  vpc_id = aws_vpc.application.id
  route {
    ipv6_cidr_block        = "::/0"
    egress_only_gateway_id = aws_egress_only_internet_gateway.application.id
  }
  tags = { Name = "${local.name}-private" }
}
resource "aws_route_table_association" "private" {
  count          = 2
  subnet_id      = aws_subnet.private[count.index].id
  route_table_id = aws_route_table.private.id
}
resource "aws_security_group" "application" {
  name        = "${local.name}-application"
  description = "API and migration Lambda egress"
  vpc_id      = aws_vpc.application.id
  tags        = { Name = "${local.name}-application" }
}
resource "aws_security_group" "database" {
  name        = "${local.name}-database"
  description = "PostgreSQL reachable only from application Lambda"
  vpc_id      = aws_vpc.application.id
  tags        = { Name = "${local.name}-database" }
}
resource "aws_vpc_security_group_ingress_rule" "postgres" {
  security_group_id            = aws_security_group.database.id
  referenced_security_group_id = aws_security_group.application.id
  ip_protocol                  = "tcp"
  from_port                    = 5432
  to_port                      = 5432
}
resource "aws_vpc_security_group_egress_rule" "postgres" {
  security_group_id            = aws_security_group.application.id
  referenced_security_group_id = aws_security_group.database.id
  ip_protocol                  = "tcp"
  from_port                    = 5432
  to_port                      = 5432
}
resource "aws_vpc_security_group_egress_rule" "https" {
  security_group_id = aws_security_group.application.id
  cidr_ipv6         = "::/0"
  ip_protocol       = "tcp"
  from_port         = 443
  to_port           = 443
}
resource "aws_db_subnet_group" "application" {
  name       = local.name
  subnet_ids = aws_subnet.private[*].id
  tags       = { Name = local.name }
}
resource "aws_db_instance" "application" {
  identifier                      = "${local.name}-postgres"
  engine                          = "postgres"
  engine_version                  = "17"
  instance_class                  = "db.t4g.micro"
  allocated_storage               = 20
  max_allocated_storage           = 100
  storage_type                    = "gp3"
  storage_encrypted               = true
  db_name                         = "openoj"
  username                        = "openoj_admin"
  manage_master_user_password     = true
  db_subnet_group_name            = aws_db_subnet_group.application.name
  vpc_security_group_ids          = [aws_security_group.database.id]
  publicly_accessible             = false
  multi_az                        = false
  backup_retention_period         = 7
  copy_tags_to_snapshot           = true
  deletion_protection             = true
  skip_final_snapshot             = false
  final_snapshot_identifier       = "${local.name}-postgres-final"
  auto_minor_version_upgrade      = true
  enabled_cloudwatch_logs_exports = ["postgresql", "upgrade"]
  depends_on                      = [aws_cloudwatch_log_group.postgres]
  lifecycle { prevent_destroy = true }
}
resource "aws_cloudwatch_log_group" "postgres" {
  for_each          = toset(["postgresql", "upgrade"])
  name              = "/aws/rds/instance/${local.name}-postgres/${each.key}"
  retention_in_days = 14
}

locals {
  database_environment = {
    DATABASE_HOST       = aws_db_instance.application.address
    DATABASE_NAME       = aws_db_instance.application.db_name
    DATABASE_SECRET_ARN = aws_db_instance.application.master_user_secret[0].secret_arn
  }
}
resource "aws_iam_role_policy" "database" {
  name = "database-credentials"
  role = aws_iam_role.api.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = ["secretsmanager:GetSecretValue"]
      Resource = aws_db_instance.application.master_user_secret[0].secret_arn
    }]
  })
}
# Lambda requires these ENI actions for VPC attachment, including on cold starts.
resource "aws_iam_role_policy" "vpc" {
  name = "vpc-interfaces"
  role = aws_iam_role.api.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = ["ec2:CreateNetworkInterface", "ec2:DescribeNetworkInterfaces", "ec2:DescribeSubnets", "ec2:DeleteNetworkInterface", "ec2:AssignPrivateIpAddresses", "ec2:UnassignPrivateIpAddresses"]
      Resource = "*"
    }]
  })
}
