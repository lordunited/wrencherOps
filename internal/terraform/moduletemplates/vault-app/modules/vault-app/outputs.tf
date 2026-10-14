output "secret_path" {
  description = "Where the app reads its secret: vault kv get -mount=<mount> <name>."
  value       = "${var.kv_mount}/${var.name}"
}

output "role_id" {
  description = "The app's AppRole RoleID (not a secret on its own)."
  value       = vault_approle_auth_backend_role.this.role_id
}

output "secret_id" {
  description = "The app's AppRole SecretID: with the RoleID, it logs the app in."
  value       = vault_approle_auth_backend_role_secret_id.this.secret_id
  sensitive   = true
}
