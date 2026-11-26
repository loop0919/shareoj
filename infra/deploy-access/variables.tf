variable "project_name" { default = "judge" }
variable "environment" { default = "dev" }
variable "aws_region" { default = "ap-northeast-1" }
variable "state_bucket" { type = string }
variable "github_subject" {
  description = "Exact GitHub OIDC subject, including immutable IDs where required."
  type        = string
  validation {
    condition     = can(regex("^repo:[^*]+:environment:dev$", var.github_subject))
    error_message = "Use the exact dev environment subject without wildcards."
  }
}
variable "github_cost_report_subject" {
  description = "Exact GitHub OIDC subject of the cost-report environment, which reads costs for the public running-cost page."
  type        = string
  validation {
    condition     = can(regex("^repo:[^*]+:environment:cost-report$", var.github_cost_report_subject))
    error_message = "Use the exact cost-report environment subject without wildcards."
  }
}
variable "api_execution_role_name" { type = string }
variable "gateway_ids" { type = set(string) }
variable "user_pool_ids" { type = set(string) }
