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
run "isolated_worker" {
  command = apply
  assert {
    condition     = strcontains(file("${path.module}/user-data.sh.tftpl"), "ip daddr 169.254.169.254 tcp dport 80 accept")
    error_message = "Host cloud-init must retain access to IMDS across reboot."
  }
  assert {
    condition     = aws_lightsail_instance.worker[0].bundle_id == "small_ipv6_3_0" && aws_lightsail_instance.worker[0].ip_address_type == "ipv6"
    error_message = "Worker must use the 2 GB IPv6-only bundle."
  }
  assert {
    condition = alltrue([for port in aws_lightsail_instance_public_ports.worker[0].port_info :
      port.protocol == "icmpv6" || (port.protocol == "tcp" && port.from_port == 22 && port.to_port == 22 && port.ipv6_cidrs == toset([var.admin_ipv6_cidr]))
    ])
    error_message = "Only operator IPv6 SSH and ICMPv6 may be exposed."
  }
  assert {
    condition     = !strcontains(aws_iam_user_policy.worker.policy, "secretsmanager") && !strcontains(aws_iam_user_policy.worker.policy, "s3:ListBucket")
    error_message = "Worker must not access database credentials or mutate job objects."
  }
  assert {
    condition = alltrue([for statement in jsondecode(aws_iam_user_policy.worker.policy).Statement :
      !contains(statement.Action, "s3:PutObject") || statement.Resource == "arn:aws:s3:::judge-test-data/test-files/*/*/generated/*"
    ])
    error_message = "Worker writes must be limited to generated test files, never job objects or manually uploaded tests."
  }
  assert {
    condition = (
      strcontains(aws_iam_user_policy.worker.policy, "s3:GetObjectVersion") &&
      strcontains(aws_iam_user_policy.worker.policy, "arn:aws:s3:::judge-test-data/test-files/*") &&
      strcontains(output.worker_environment, "JUDGE_TEST_DATA_BUCKET=judge-test-data")
    )
    error_message = "Worker must read immutable test-data versions and receive the test-data bucket name."
  }
  assert {
    condition     = aws_cloudwatch_event_rule.dispatch.state == "DISABLED" && !aws_lambda_event_source_mapping.results.enabled
    error_message = "Queue integration must be opt-in after migration and smoke tests."
  }
  assert {
    condition = (
      aws_sqs_queue.queue["requests"].visibility_timeout_seconds > 1800 &&
      aws_sqs_queue.queue["results"].visibility_timeout_seconds >= 6 * aws_lambda_function.bridge.timeout &&
      aws_lambda_function.bridge.reserved_concurrent_executions == 5
    )
    error_message = "Queue leases must cover deadlines and bridge concurrency must protect the database."
  }
}
run "enable_dispatch" {
  command = plan
  variables { enabled = true }
  assert {
    condition     = aws_cloudwatch_event_rule.dispatch.state == "ENABLED" && aws_lambda_event_source_mapping.results.enabled
    error_message = "Enabling the integration must activate dispatch and result consumption together."
  }
}

run "reject_unpinned_enabled_runtime" {
  command = plan
  variables {
    enabled        = true
    runtime_digest = ""
  }
  expect_failures = [aws_lambda_function.bridge]
}
run "reject_public_ssh" {
  command = plan
  variables { admin_ipv6_cidr = "::/0" }
  expect_failures = [var.admin_ipv6_cidr]
}
run "ssm_only" {
  command = plan
  variables { ssh_enabled = false }
  assert {
    condition     = alltrue([for port in aws_lightsail_instance_public_ports.worker[0].port_info : port.protocol == "icmpv6"])
    error_message = "SSM-only mode must expose no TCP ports."
  }
}

run "two_workers" {
  command = plan
  variables { worker_count = 2 }
  assert {
    condition = (
      length(aws_lightsail_instance.worker) == 2 &&
      length(aws_lightsail_instance_public_ports.worker) == 2 &&
      aws_lightsail_instance.worker[0].name == "judge-dev-judge-worker" &&
      aws_lightsail_instance.worker[1].name == "judge-dev-judge-worker-2"
    )
    error_message = "Two workers must retain the original host name and receive separate firewalls."
  }
  assert {
    condition = (
      aws_cloudwatch_metric_alarm.worker["judge-dev-judge-worker"].dimensions.Worker == "judge-dev-judge-worker" &&
      aws_cloudwatch_metric_alarm.worker["judge-dev-judge-worker-2"].dimensions.Worker == "judge-dev-judge-worker-2" &&
      jsondecode(output.cloudwatch_agent_configs["judge-dev-judge-worker-2"]).metrics.metrics_collected.procstat[0].append_dimensions.Worker == "judge-dev-judge-worker-2"
    )
    error_message = "Each worker needs its own process heartbeat and missing-data alarm."
  }
}

run "retire_lightsail" {
  command = plan
  variables { worker_count = 0 }
  assert {
    condition = (
      length(aws_lightsail_instance.worker) == 0 && output.worker_instance_name == null &&
      keys(aws_cloudwatch_metric_alarm.worker) == ["judge-dev-judge-burst-1", "judge-dev-judge-burst-2", "judge-dev-judge-primary"]
    )
    error_message = "Retiring Lightsail must leave only the EC2 pool and its heartbeats."
  }
}

run "reject_too_many_workers" {
  command = plan
  variables { worker_count = 7 }
  expect_failures = [var.worker_count]
}

run "observability" {
  command = plan
  variables {
    alerts_enabled             = true
    discord_webhook_secret_arn = "arn:aws:secretsmanager:ap-northeast-1:123456789012:secret:discord-test"
    discord_webhook_secret_key = "ALART_DISCORD_WEBHOOK"
  }
  assert {
    condition = (
      aws_cloudwatch_metric_alarm.worker["judge-dev-judge-worker"].treat_missing_data == "breaching" &&
      aws_cloudwatch_metric_alarm.worker["judge-dev-judge-primary"].treat_missing_data == "breaching" &&
      aws_cloudwatch_metric_alarm.worker["judge-dev-judge-primary"].evaluation_periods == 3 &&
      aws_cloudwatch_metric_alarm.judge["dispatch-missing"].treat_missing_data == "breaching"
    )
    error_message = "Always-on workers and scheduled DB observation must detect missing telemetry."
  }
  assert {
    condition     = alltrue([for name in ["judge-dev-judge-burst-1", "judge-dev-judge-burst-2"] : aws_cloudwatch_metric_alarm.worker[name].treat_missing_data == "notBreaching"])
    error_message = "A stopped burst host is expected to send no heartbeat."
  }
  assert {
    condition = (
      aws_cloudwatch_metric_alarm.worker["judge-dev-judge-worker"].actions_enabled &&
      !aws_cloudwatch_metric_alarm.worker["judge-dev-judge-primary"].actions_enabled &&
      aws_cloudwatch_metric_alarm.judge["platform"].actions_enabled
    )
    error_message = "Pool heartbeats stay silent until the pool serves traffic, without muting existing alarms."
  }
  assert {
    condition     = aws_cloudwatch_metric_alarm.judge["judge-code"].metric_name != aws_cloudwatch_metric_alarm.judge["platform"].metric_name && aws_cloudwatch_metric_alarm.judge["judge-code"].treat_missing_data == "notBreaching"
    error_message = "Author errors must be separate from infrastructure errors; inactivity is healthy."
  }
  assert {
    condition     = aws_cloudwatch_log_group.worker.retention_in_days == 14 && length(aws_cloudwatch_metric_alarm.dead["requests"].alarm_actions) == 1 && length(aws_cloudwatch_metric_alarm.dead["results"].ok_actions) == 1
    error_message = "Bound retention and connect existing DLQ alarm/recovery actions."
  }
  assert {
    condition     = jsondecode(aws_iam_user_policy.worker_observability.policy).Statement[1].Condition.StringEquals["cloudwatch:namespace"] == "Judge/judge-dev" && !strcontains(aws_iam_user_policy.worker_observability.policy, "secretsmanager")
    error_message = "Worker metrics must be namespace restricted and cannot read notification secrets."
  }
  assert {
    condition     = aws_lambda_function.notify.environment[0].variables.WEBHOOK_SECRET_KEY == "ALART_DISCORD_WEBHOOK" && length(aws_lambda_function.notify.vpc_config) == 0
    error_message = "Notifier reads the selected secret key and requires no NAT or judge host network."
  }
}
run "pool_alerts" {
  command = plan
  variables {
    alerts_enabled             = true
    pool_alerts_enabled        = true
    discord_webhook_secret_arn = "arn:aws:secretsmanager:ap-northeast-1:123456789012:secret:discord-test"
  }
  assert {
    condition     = alltrue([for alarm in aws_cloudwatch_metric_alarm.worker : alarm.actions_enabled])
    error_message = "After the cutover every host heartbeat notifies."
  }
}
run "reject_alerts_without_secret" {
  command = plan
  variables { alerts_enabled = true }
  expect_failures = [aws_lambda_function.notify]
}

run "pool_hosts" {
  command = apply
  assert {
    condition = (
      keys(aws_instance.pool) == ["judge-dev-judge-burst-1", "judge-dev-judge-burst-2", "judge-dev-judge-primary"] &&
      aws_instance.pool["judge-dev-judge-primary"].tags.JudgeRole == "primary" &&
      aws_instance.pool["judge-dev-judge-burst-1"].tags.JudgeRole == "burst" &&
      alltrue([for host in aws_instance.pool : host.tags.JudgePool == "judge-dev" && host.tags.Component == "judge-worker"])
    )
    error_message = "The pool must have one primary and the configured burst hosts, tagged for the bridge."
  }
  assert {
    condition = alltrue([for host in aws_instance.pool : (
      host.instance_type == "t3.small" && host.credit_specification[0].cpu_credits == "unlimited" &&
      host.metadata_options[0].http_tokens == "required" && host.metadata_options[0].http_put_response_hop_limit == 1 &&
      host.metadata_options[0].http_protocol_ipv6 == "enabled" && host.metadata_options[0].instance_metadata_tags == "enabled" &&
      !host.associate_public_ip_address && host.ipv6_address_count == 1 && host.instance_initiated_shutdown_behavior == "stop" &&
      host.root_block_device[0].encrypted && host.root_block_device[0].volume_type == "gp3" && host.root_block_device[0].volume_size == 60
    )])
    error_message = "Hosts must be unthrottled, IPv6-only, IMDSv2-only, stop on shutdown and hold an upgrade on disk."
  }
  assert {
    condition = (
      alltrue([for subnet in aws_subnet.pool : subnet.ipv6_native]) &&
      aws_vpc_security_group_egress_rule.pool_https.cidr_ipv6 == "::/0" && aws_vpc_security_group_egress_rule.pool_https.from_port == 443 &&
      one(aws_route_table.pool.route).ipv6_cidr_block == "::/0" && one(aws_route_table.pool.route).egress_only_gateway_id != ""
    )
    error_message = "The pool network must be IPv6-only with outbound HTTPS through an egress-only gateway."
  }
  assert {
    condition = (
      strcontains(file("${path.module}/user-data-ec2.sh.tftpl"), "ip6 daddr fd00:ec2::254 tcp dport 80 meta skuid 0 accept") &&
      strcontains(file("${path.module}/user-data-ec2.sh.tftpl"), "ip daddr 169.254.169.254 tcp dport 80 meta skuid 0 accept") &&
      !strcontains(file("${path.module}/user-data-ec2.sh.tftpl"), "dport 22")
    )
    error_message = "Only root may reach IMDS, and pool hosts accept no SSH."
  }
  assert {
    condition = (
      strcontains(output.pool_worker_environments["judge-dev-judge-primary"], "JUDGE_POOL_ROLE=primary") &&
      strcontains(output.pool_worker_environments["judge-dev-judge-burst-2"], "JUDGE_POOL_ROLE=burst") &&
      alltrue([for env in output.pool_worker_environments : (
        strcontains(env, "AWS_EC2_METADATA_SERVICE_ENDPOINT_MODE=IPv6") && strcontains(env, "JUDGE_TEST_DATA_BUCKET=judge-test-data") &&
        !strcontains(env, "AWS_SHARED_CREDENTIALS_FILE") && !strcontains(env, "AWS_EC2_METADATA_DISABLED")
      )])
    )
    error_message = "Pool workers use instance-role credentials over IPv6 IMDS and know their role."
  }
  assert {
    condition = (
      !strcontains(aws_iam_role_policy.pool.policy, "secretsmanager") && !strcontains(aws_iam_role_policy.pool.policy, "ssm:GetParameter") &&
      strcontains(aws_iam_role_policy.pool.policy, "ssm:UpdateInstanceInformation") && strcontains(aws_iam_role_policy.pool.policy, "sqs:ReceiveMessage") &&
      strcontains(aws_iam_role_policy.pool.policy, "cloudwatch:namespace")
    )
    error_message = "The instance role carries worker, monitoring and SSM agent access only."
  }
  assert {
    condition = (
      anytrue([for s in jsondecode(aws_iam_role_policy.bridge.policy).Statement : contains(s.Action, "ec2:StartInstances")]) &&
      alltrue([for s in jsondecode(aws_iam_role_policy.bridge.policy).Statement :
        !(contains(s.Action, "ec2:StartInstances") || contains(s.Action, "ec2:CreateTags")) ||
        try(s.Condition.StringEquals["aws:ResourceTag/JudgePool"], "") == "judge-dev"
      ]) &&
      alltrue([for s in jsondecode(aws_iam_role_policy.bridge.policy).Statement :
        !contains(s.Action, "ec2:CreateTags") || try(s.Condition["ForAllValues:StringEquals"]["aws:TagKeys"], []) == ["JudgeHoldUntil"]
      ]) &&
      !strcontains(aws_iam_role_policy.bridge.policy, "StopInstances") && !strcontains(aws_iam_role_policy.bridge.policy, "TerminateInstances")
    )
    error_message = "The bridge may only start pool hosts and extend their contest hold."
  }
  assert {
    condition = (
      aws_lambda_function.bridge.environment[0].variables.JUDGE_CAPACITY_ENABLED == "false" &&
      aws_lambda_function.bridge.environment[0].variables.JUDGE_POOL == "judge-dev" &&
      aws_lambda_function.bridge.environment[0].variables.JUDGE_BURST_MIN_PARTICIPANTS == "10" &&
      aws_lambda_function.bridge.environment[0].variables.JUDGE_ENABLED_RUNTIMES == "cpp17,python314"
    )
    error_message = "Capacity control is opt-in, and the published runtimes survive a Terraform apply."
  }
}

run "no_burst_hosts" {
  command = plan
  variables { burst_worker_count = 0 }
  assert {
    condition     = keys(aws_instance.pool) == ["judge-dev-judge-primary"]
    error_message = "The primary host exists without burst hosts."
  }
}

run "reject_capacity_without_runtime" {
  command = plan
  variables {
    capacity_enabled = true
    runtime_digest   = ""
  }
  expect_failures = [aws_lambda_function.bridge]
}

run "reject_missing_runtimes" {
  command = plan
  variables { enabled_runtimes = [] }
  expect_failures = [var.enabled_runtimes]
}
