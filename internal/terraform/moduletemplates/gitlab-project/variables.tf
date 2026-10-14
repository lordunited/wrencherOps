variable "gitlab_base_url" {
  type        = string
  description = "GitLab's API address, e.g. https://gitlab.corp.local/api/v4/."
  default     = "https://gitlab.com/api/v4/"
}

variable "gitlab_token" {
  type        = string
  description = "A GitLab access token with the api scope."
  sensitive   = true
}

variable "projects" {
  type = map(object({
    group_path          = string
    description         = optional(string, "")
    visibility          = optional(string, "private")
    default_branch      = optional(string, "main")
    require_pipeline    = optional(bool, true)
    ci_variables        = optional(map(string), {})
    deploy_token_scopes = optional(list(string), [])
  }))
  description = "The projects to create, by name. Edit them in projects.auto.tfvars."
}

variable "project_ci_secrets" {
  type        = map(map(string))
  description = "Secret CI/CD variables per project, e.g. { billing-api = { REGISTRY_PASSWORD = \"...\" } }. Set it on the Variables tab, never in the code."
  sensitive   = true
  default     = {}
}
