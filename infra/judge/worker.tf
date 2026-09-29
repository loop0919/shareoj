# Lightsail hosts remain until the EC2 pool (ec2.tf) takes over; set worker_count = 0 to retire them.
# No peering to the application VPC and no database credentials on the worker.
locals {
  # Shared by the Lightsail IAM user and the EC2 instance role.
  worker_statements = concat([
    { Effect = "Allow", Action = ["sqs:ReceiveMessage", "sqs:DeleteMessage", "sqs:ChangeMessageVisibility"], Resource = aws_sqs_queue.queue["requests"].arn },
    { Effect = "Allow", Action = ["sqs:SendMessage"], Resource = aws_sqs_queue.queue["results"].arn },
    { Effect = "Allow", Action = ["s3:GetObjectVersion"], Resource = concat(["${aws_s3_bucket.jobs.arn}/jobs/*"], var.test_data_bucket == null ? [] : ["${var.test_data_bucket.arn}/test-files/*"]) }
    ], var.test_data_bucket == null ? [] : [
    { Effect = "Allow", Action = ["s3:PutObject", "s3:PutObjectTagging"], Resource = "${var.test_data_bucket.arn}/test-files/*/*/generated/*" }
  ])
}
resource "aws_lightsail_key_pair" "worker" {
  name       = "${local.name}-judge"
  public_key = var.ssh_public_key
}
resource "aws_lightsail_instance" "worker" {
  count             = var.worker_count
  name              = count.index == 0 ? "${local.name}-judge-worker" : "${local.name}-judge-worker-${count.index + 1}"
  availability_zone = var.availability_zone
  blueprint_id      = "ubuntu_24_04"
  bundle_id         = "small_ipv6_3_0"
  ip_address_type   = "ipv6"
  key_pair_name     = aws_lightsail_key_pair.worker.name
  # Bootstrap contains no credentials. The release is uploaded over SSH.
  user_data = "bash -c 'echo ${base64encode(templatefile("${path.module}/user-data.sh.tftpl", { admin_ipv6_cidr = var.admin_ipv6_cidr }))} | base64 -d | bash'"
  lifecycle {
    # Snapshot clones are imported after enrollment; bootstrap edits must not replace enrolled hosts.
    ignore_changes = [user_data]
  }
}
resource "aws_lightsail_instance_public_ports" "worker" {
  count         = var.worker_count
  instance_name = aws_lightsail_instance.worker[count.index].name
  lifecycle {
    # A same-name replacement resets Lightsail's firewall to blueprint defaults.
    replace_triggered_by = [aws_lightsail_instance.worker[count.index]]
  }
  dynamic "port_info" {
    for_each = var.ssh_enabled ? [1] : []
    content {
      protocol   = "tcp"
      from_port  = 22
      to_port    = 22
      cidrs      = []
      ipv6_cidrs = [var.admin_ipv6_cidr]
    }
  }
  port_info {
    protocol   = "icmpv6"
    from_port  = -1
    to_port    = -1
    cidrs      = []
    ipv6_cidrs = ["::/0"]
  }
}

moved {
  from = aws_lightsail_instance.worker
  to   = aws_lightsail_instance.worker[0]
}
moved {
  from = aws_lightsail_instance_public_ports.worker
  to   = aws_lightsail_instance_public_ports.worker[0]
}

# Lightsail is enrolled as a hybrid managed node. Activation credentials are
# generated once outside Terraform and passed over the verified SSH channel.
resource "aws_iam_role" "worker_ssm" {
  name = "${local.name}-judge-ssm"
  assume_role_policy = jsonencode({
    Version   = "2012-10-17"
    Statement = [{ Effect = "Allow", Principal = { Service = "ssm.amazonaws.com" }, Action = "sts:AssumeRole" }]
  })
}
resource "aws_iam_role_policy_attachment" "worker_ssm" {
  role       = aws_iam_role.worker_ssm.name
  policy_arn = "arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore"
}
# Lightsail has no EC2 instance profile. Create/rotate the restricted access key
# outside Terraform; install it into /root/.aws/credentials with mode 0600.
resource "aws_iam_user" "worker" {
  name = "${local.name}-judge-worker"
}
resource "aws_iam_user_policy" "worker" {
  user   = aws_iam_user.worker.name
  policy = jsonencode({ Version = "2012-10-17", Statement = local.worker_statements })
}
