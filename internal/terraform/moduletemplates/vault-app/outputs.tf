output "secret_paths" {
  description = "Where each app reads its secret."
  value       = { for name, app in module.app : name => app.secret_path }
}

output "role_ids" {
  description = "Each app's AppRole RoleID."
  value       = { for name, app in module.app : name => app.role_id }
}

output "secret_ids" {
  description = "Each app's AppRole SecretID, to hand to the app with its RoleID."
  value       = { for name, app in module.app : name => app.secret_id }
  sensitive   = true
}
