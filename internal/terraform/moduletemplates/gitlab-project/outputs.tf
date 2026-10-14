output "web_urls" {
  description = "Each project's page."
  value       = { for name, p in module.project : name => p.web_url }
}

output "deploy_tokens" {
  description = "Each project's deploy token, when it has one."
  value       = { for name, p in module.project : name => p.deploy_token }
  sensitive   = true
}
