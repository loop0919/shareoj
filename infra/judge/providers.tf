provider "aws" {
  region = var.aws_region
  # Runtime tags written by the bridge and rollout, not by Terraform.
  ignore_tags {
    keys = ["JudgeHoldUntil", "JudgeMaintenanceHoldUntil", "JudgeInstalledDigest"]
  }
  default_tags {
    tags = {
      Project     = var.project_name
      Service     = var.project_name
      Environment = title(var.environment)
      ManagedBy   = "Terraform"
    }
  }
}

locals {
  name = "${var.project_name}-${var.environment}"
}
