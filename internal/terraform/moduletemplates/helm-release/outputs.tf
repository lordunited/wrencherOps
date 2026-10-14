output "statuses" {
  description = "Each release's status."
  value       = { for name, r in module.release : name => r.status }
}
