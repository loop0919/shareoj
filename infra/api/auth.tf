locals {
  # The provider cannot clear an imported default redirect (null preserves it;
  # empty strings are rejected). OAuth-off uses a non-resolving reserved domain.
  cognito_callback_url = var.google_client_id != "" ? "${var.public_site_url}/auth/google/callback" : "https://disabled.invalid/auth/google/callback"
}

resource "aws_cognito_user_pool" "users" {
  name                     = "${local.name}-users"
  user_pool_tier           = "LITE"
  username_attributes      = ["email"]
  auto_verified_attributes = ["email"]
  deletion_protection      = "ACTIVE"
  mfa_configuration        = "OFF"

  username_configuration {
    case_sensitive = false
  }

  admin_create_user_config {
    allow_admin_create_user_only = false
  }

  verification_message_template {
    default_email_option = "CONFIRM_WITH_CODE"
  }

  password_policy {
    minimum_length                   = 12
    require_lowercase                = true
    require_uppercase                = true
    require_numbers                  = true
    require_symbols                  = true
    temporary_password_validity_days = 7
  }

  account_recovery_setting {
    recovery_mechanism {
      name     = "verified_email"
      priority = 1
    }
  }

  lifecycle {
    prevent_destroy = true
  }
}

resource "aws_cognito_user_pool_client" "api" {
  name                          = "${local.name}-api"
  user_pool_id                  = aws_cognito_user_pool.users.id
  generate_secret               = true
  explicit_auth_flows           = ["ALLOW_USER_PASSWORD_AUTH", "ALLOW_REFRESH_TOKEN_AUTH"]
  prevent_user_existence_errors = "ENABLED"
  enable_token_revocation       = true
  auth_session_validity         = 3
  access_token_validity         = 60
  id_token_validity             = 60
  refresh_token_validity        = 30
  read_attributes               = ["email", "email_verified"]
  write_attributes              = ["email"]

  supported_identity_providers         = var.google_client_id != "" ? ["COGNITO", "Google"] : ["COGNITO"]
  allowed_oauth_flows_user_pool_client = var.google_client_id != ""
  allowed_oauth_flows                  = var.google_client_id != "" ? ["code"] : []
  allowed_oauth_scopes                 = var.google_client_id != "" ? ["openid", "email"] : []
  # Manage the local callback too, so apply preserves local Google login in dev.
  callback_urls = concat(
    [local.cognito_callback_url],
    var.environment == "dev" && var.google_client_id != "" ? ["http://localhost:3000/auth/google/callback"] : []
  )
  # Logout ends Cognito's own sign-in cookie at /logout, which returns only to these pages.
  logout_urls = var.google_client_id != "" ? concat(
    ["${var.public_site_url}/login"],
    var.environment == "dev" ? ["http://localhost:3000/login"] : []
  ) : []
  # Keep the default and allowlist consistent when adopting existing clients.
  default_redirect_uri = local.cognito_callback_url
  depends_on           = [aws_cognito_identity_provider.google]

  token_validity_units {
    access_token  = "minutes"
    id_token      = "minutes"
    refresh_token = "days"
  }
}
