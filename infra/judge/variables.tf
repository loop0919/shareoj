variable "aws_region" {
  type    = string
  default = "ap-northeast-1"
}
variable "project_name" {
  type    = string
  default = "judge"
}
variable "environment" {
  type    = string
  default = "dev"
  validation {
    condition     = contains(["dev", "stage", "prod"], var.environment)
    error_message = "environment must be dev, stage or prod."
  }
}
variable "availability_zone" {
  type    = string
  default = "ap-northeast-1a"
}
variable "worker_count" {
  description = "Number of Lightsail single-slot workers. Set 0 after the EC2 pool takes over."
  type        = number
  default     = 1
  validation {
    condition     = var.worker_count >= 0 && var.worker_count <= 6 && floor(var.worker_count) == var.worker_count
    error_message = "worker_count must be an integer from 0 to 6."
  }
}
variable "burst_worker_count" {
  description = "Normally stopped EC2 hosts that the bridge starts for contests, besides the always-on primary."
  type        = number
  default     = 2
  validation {
    condition     = var.burst_worker_count >= 0 && var.burst_worker_count <= 4 && floor(var.burst_worker_count) == var.burst_worker_count
    error_message = "burst_worker_count must be an integer from 0 to 4."
  }
}
variable "worker_availability_zones" {
  description = "Zones offering t3a.small; check with describe-instance-type-offerings. Hosts are spread in order."
  type        = list(string)
  default     = ["ap-northeast-1a", "ap-northeast-1d"]
  validation {
    condition     = length(var.worker_availability_zones) >= 1 && length(var.worker_availability_zones) <= 3
    error_message = "Specify one to three availability zones."
  }
}
variable "worker_volume_size" {
  description = "Root gp3 size in GB. An upgrade holds the current runtime (about 20 GB), its archive and the staged runtime at once."
  type        = number
  default     = 60
  validation {
    condition     = var.worker_volume_size >= 30 && var.worker_volume_size <= 200
    error_message = "worker_volume_size must be 30 to 200 GB."
  }
}
variable "pool_alerts_enabled" {
  description = "Notify for EC2 host heartbeats. Keep false until the pool serves traffic; the primary alarms while its worker is not installed."
  type        = bool
  default     = false
}
variable "capacity_enabled" {
  description = "Let the bridge start EC2 hosts for contests. Enable after the pool serves traffic."
  type        = bool
  default     = false
}
variable "burst_min_participants" {
  description = "Registered participants a contest needs before burst hosts start for it."
  type        = number
  default     = 10
  validation {
    condition     = var.burst_min_participants >= 1 && floor(var.burst_min_participants) == var.burst_min_participants
    error_message = "burst_min_participants must be a positive integer."
  }
}
variable "enabled_runtimes" {
  description = "Published runtime IDs, kept in sync by judge/rollout.py. Required so an apply never resets the bridge."
  type        = list(string)
  validation {
    condition     = length(var.enabled_runtimes) > 0 && alltrue([for id in var.enabled_runtimes : can(regex("^[a-z][a-z0-9-]*$", id))])
    error_message = "enabled_runtimes must list published runtime IDs."
  }
}
variable "ssh_public_key" {
  description = "Operator SSH public key; private keys are never stored in Terraform."
  type        = string
}
variable "admin_ipv6_cidr" {
  description = "Operator's IPv6 /128 address for SSH. IPv6 connectivity is required."
  type        = string
  validation {
    condition     = can(cidrhost(var.admin_ipv6_cidr, 0)) && endswith(var.admin_ipv6_cidr, "/128") && strcontains(var.admin_ipv6_cidr, ":")
    error_message = "Specify one operator IPv6 address with /128."
  }
}
variable "ssh_enabled" {
  description = "Bootstrap/recovery SSH from admin_ipv6_cidr. Disable after SSM connectivity and reboot verification."
  type        = bool
  default     = true
}
variable "bridge_package_path" {
  type    = string
  default = "../../api/.build/judge-bridge.zip"
}
variable "runtime_digest" {
  description = "Digest printed by install.sh on the worker after installing and fingerprinting the runtime. Empty until installation."
  type        = string
  default     = ""
  validation {
    condition     = var.runtime_digest == "" || can(regex("^sha256:[a-f0-9]{64}$", var.runtime_digest))
    error_message = "runtime_digest must be sha256 followed by 64 hex characters."
  }
}
variable "database" {
  description = "infra/api judge_bridge_database output. Used only by the bridge Lambda."
  type = object({
    host              = string
    name              = string
    secret_arn        = string
    subnet_ids        = list(string)
    security_group_id = string
  })
}
variable "test_data_bucket" {
  description = "infra/api test_data_bucket output. The worker can only read immutable versions."
  type = object({
    id  = string
    arn = string
  })
  default  = null
  nullable = true
}
variable "enabled" {
  description = "Start dispatcher after migrations and worker smoke tests pass."
  type        = bool
  default     = false
}
