output "web_url" {
  description = "The project's page."
  value       = gitlab_project.this.web_url
}

output "ssh_url" {
  description = "The project's SSH clone URL."
  value       = gitlab_project.this.ssh_url_to_repo
}

output "deploy_token_username" {
  description = "The deploy token's username, when there is one."
  value       = try(gitlab_project_deploy_token.this[0].username, null)
}

output "deploy_token" {
  description = "The deploy token itself, when there is one."
  value       = try(gitlab_project_deploy_token.this[0].token, null)
  sensitive   = true
}
