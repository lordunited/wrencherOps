resource "vault_kv_secret_v2" "this" {
  mount     = var.kv_mount
  name      = var.name
  data_json = jsonencode(var.secrets)
}

# Read access to the app's own secret, nothing else.
resource "vault_policy" "read" {
  name   = "${var.name}-read"
  policy = <<-EOT
    path "${var.kv_mount}/data/${var.name}" {
      capabilities = ["read"]
    }

    path "${var.kv_mount}/metadata/${var.name}" {
      capabilities = ["read", "list"]
    }
  EOT
}

resource "vault_approle_auth_backend_role" "this" {
  backend               = var.approle_backend
  role_name             = var.name
  token_policies        = concat([vault_policy.read.name], var.extra_policies)
  token_ttl             = var.token_ttl
  token_max_ttl         = var.token_max_ttl
  secret_id_ttl         = var.secret_id_ttl
  token_bound_cidrs     = var.bound_cidrs
  secret_id_bound_cidrs = var.bound_cidrs
}

resource "vault_approle_auth_backend_role_secret_id" "this" {
  backend   = var.approle_backend
  role_name = vault_approle_auth_backend_role.this.role_name
}
