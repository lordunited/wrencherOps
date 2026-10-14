output "status" {
  description = "The release's status, e.g. deployed."
  value       = helm_release.this.status
}

output "chart_version" {
  description = "The chart version installed."
  value       = helm_release.this.version
}
