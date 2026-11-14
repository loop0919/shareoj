resource "aws_s3_bucket" "jobs" {
  bucket_prefix = "${local.name}-judge-"
}
resource "aws_s3_bucket_versioning" "jobs" {
  bucket = aws_s3_bucket.jobs.id
  versioning_configuration { status = "Enabled" }
}
resource "aws_s3_bucket_public_access_block" "jobs" {
  bucket                  = aws_s3_bucket.jobs.id
  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}
resource "aws_s3_bucket_ownership_controls" "jobs" {
  bucket = aws_s3_bucket.jobs.id
  rule { object_ownership = "BucketOwnerEnforced" }
}
resource "aws_s3_bucket_server_side_encryption_configuration" "jobs" {
  bucket = aws_s3_bucket.jobs.id
  rule {
    apply_server_side_encryption_by_default { sse_algorithm = "AES256" }
  }
}
resource "aws_s3_bucket_policy" "jobs" {
  bucket = aws_s3_bucket.jobs.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Deny", Principal = "*", Action = "s3:*"
      Resource  = [aws_s3_bucket.jobs.arn, "${aws_s3_bucket.jobs.arn}/*"]
      Condition = { Bool = { "aws:SecureTransport" = "false" } }
    }]
  })
}
resource "aws_s3_bucket_lifecycle_configuration" "jobs" {
  bucket = aws_s3_bucket.jobs.id
  rule {
    id     = "expire-job-transfers"
    status = "Enabled"
    filter { prefix = "jobs/" }
    expiration { days = 7 }
    noncurrent_version_expiration { noncurrent_days = 7 }
    abort_incomplete_multipart_upload { days_after_initiation = 1 }
  }
}
resource "aws_s3_object" "bridge" {
  bucket      = aws_s3_bucket.jobs.id
  key         = "releases/${filesha256(var.bridge_package_path)}/bridge.zip"
  source      = var.bridge_package_path
  source_hash = filesha256(var.bridge_package_path)
  depends_on  = [aws_s3_bucket_versioning.jobs]
}
resource "aws_sqs_queue" "dead" {
  for_each                  = toset(["requests", "results"])
  name                      = "${local.name}-judge-${each.key}-dead"
  sqs_managed_sse_enabled   = true
  message_retention_seconds = 1209600
}
resource "aws_sqs_queue" "queue" {
  # requests = lane a, requests-b = lane b; both lanes share the requests DLQ and the results queue.
  for_each                   = toset(["requests", "requests-b", "results"])
  name                       = "${local.name}-judge-${each.key}"
  sqs_managed_sse_enabled    = true
  message_retention_seconds  = 86400
  receive_wait_time_seconds  = 20
  visibility_timeout_seconds = startswith(each.key, "requests") ? 2100 : 720
  redrive_policy = jsonencode({
    deadLetterTargetArn = aws_sqs_queue.dead[startswith(each.key, "requests") ? "requests" : "results"].arn
    maxReceiveCount     = 3
  })
  # SQS checks the DLQ's allow policy when a queue names that DLQ, so a new lane is created only after it is allowed.
  depends_on = [aws_sqs_queue_redrive_allow_policy.dead]
}
resource "aws_sqs_queue_redrive_allow_policy" "dead" {
  for_each  = aws_sqs_queue.dead
  queue_url = each.value.id
  # The source ARNs follow the queue names ("<name>-dead" serves "<name>"), so the policy does not depend on the
  # queues and can be applied before lane b exists. Lane a and the result queue keep the same ARN strings.
  redrive_allow_policy = jsonencode({
    redrivePermission = "byQueue"
    sourceQueueArns   = [for key in(each.key == "requests" ? ["requests", "requests-b"] : [each.key]) : replace(each.value.arn, "${each.key}-dead", key)]
  })
}
