output "container_ids" {
  description = "Each container's ID."
  value       = { for name, c in module.container : name => c.id }
}
