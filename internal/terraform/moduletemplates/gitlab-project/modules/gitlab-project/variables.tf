variable "name" {
  type        = string
  description = "The project's name, also its path."
}

variable "group_path" {
  type        = string
  description = "The full path of the group the project is in, e.g. platform/services."
}

variable "description" {
  type        = string
  description = "The project's description."
  default     = ""
}

variable "visibility" {
  type        = string
  description = "private, internal or public."
  default     = "private"

  validation {
    condition     = contains(["private", "internal", "public"], var.visibility)
    error_message = "visibility must be private, internal or public."
  }
}

variable "default_branch" {
  type        = string
  description = "The default branch, protected: changes reach it through merge requests only."
  default     = "main"
}

variable "require_pipeline" {
  type        = bool
  description = "Merge only when the pipeline succeeds."
  default     = true
}

variable "ci_variables" {
  type        = map(string)
  description = "CI/CD variables that aren't secret, e.g. { DEPLOY_ENV = \"staging\" }."
  default     = {}
}

variable "ci_secrets" {
  type        = map(string)
  description = "Secret CI/CD variables: masked and protected. GitLab masks only values of 8+ characters without spaces."
  sensitive   = true
  default     = {}
}

variable "deploy_token_scopes" {
  type        = list(string)
  description = "Scopes of a deploy token to create, e.g. [\"read_repository\", \"read_registry\"]. Empty for none."
  default     = []
}
