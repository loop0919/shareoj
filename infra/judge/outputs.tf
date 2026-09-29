output "worker_instance_name" { value = try(aws_lightsail_instance.worker[0].name, null) }
output "worker_ipv6_addresses" { value = try(aws_lightsail_instance.worker[0].ipv6_addresses, null) }
output "workers" {
  value = { for worker in aws_lightsail_instance.worker : worker.name => worker.ipv6_addresses }
}
output "pool_hosts" {
  description = "EC2 judge hosts; rollout nodes must list every id."
  value       = { for name, instance in aws_instance.pool : name => { id = instance.id, role = local.pool_hosts[name].role } }
}
output "pool_worker_environments" { value = local.pool_environment }
output "worker_iam_user" { value = aws_iam_user.worker.name }
output "worker_ssm_role" { value = aws_iam_role.worker_ssm.name }
output "worker_environment" {
  value = join("\n", [
    "AWS_DEFAULT_REGION=${var.aws_region}",
    "AWS_EC2_METADATA_DISABLED=true",
    "AWS_SHARED_CREDENTIALS_FILE=/root/.aws/credentials",
    "JUDGE_JOB_BUCKET=${aws_s3_bucket.jobs.id}",
    "JUDGE_TEST_DATA_BUCKET=${var.test_data_bucket == null ? "" : var.test_data_bucket.id}",
    "JUDGE_REQUEST_QUEUE_URL=${aws_sqs_queue.queue["requests"].url}",
    "JUDGE_RESULT_QUEUE_URL=${aws_sqs_queue.queue["results"].url}",
    "JUDGE_RUNTIME_DIGEST=${var.runtime_digest}",
    ""
  ])
}
output "request_queue_url" { value = aws_sqs_queue.queue["requests"].url }
output "result_queue_url" { value = aws_sqs_queue.queue["results"].url }
output "job_bucket" { value = aws_s3_bucket.jobs.id }
output "runtime_digest" { value = var.runtime_digest }
output "bridge_function_name" { value = aws_lambda_function.bridge.function_name }
