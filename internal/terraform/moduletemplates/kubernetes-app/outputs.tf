output "service_dns" {
  description = "Each app's Service name inside the cluster."
  value       = { for name, app in module.app : name => app.service_dns }
}
