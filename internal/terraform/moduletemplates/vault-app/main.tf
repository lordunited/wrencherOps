resource "vault_mount" "kv" {
  count       = var.create_kv_mount ? 1 : 0
  path        = var.kv_mount_path
  type        = "kv"
  options     = { version = "2" }
  description = "App secrets, managed by OpenTofu"
}

resource "vault_auth_backend" "approle" {
  count       = var.create_approle_backend ? 1 : 0
  type        = "approle"
  path        = var.approle_path
  description = "App logins, managed by OpenTofu"
}

# One secret, read policy and AppRole per entry of var.apps (see
# apps.auto.tfvars), each from the reusable module in modules/vault-app.
module "app" {
  source   = "./modules/vault-app"
  for_each = var.apps

  name            = each.key
  kv_mount        = var.create_kv_mount ? vault_mount.kv[0].path : var.kv_mount_path
  approle_backend = var.create_approle_backend ? vault_auth_backend.approle[0].path : var.approle_path
  secrets         = lookup(var.app_secrets, each.key, {})
  token_ttl       = each.value.token_ttl
  token_max_ttl   = each.value.token_max_ttl
  secret_id_ttl   = each.value.secret_id_ttl
  bound_cidrs     = each.value.bound_cidrs
  extra_policies  = each.value.extra_policies
}
