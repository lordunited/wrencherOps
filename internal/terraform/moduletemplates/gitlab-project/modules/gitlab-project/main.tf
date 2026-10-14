data "gitlab_group" "this" {
  full_path = var.group_path
}

resource "gitlab_project" "this" {
  name                                  = var.name
  namespace_id                          = data.gitlab_group.this.group_id
  description                           = var.description
  visibility_level                      = var.visibility
  default_branch                        = var.default_branch
  initialize_with_readme                = true
  only_allow_merge_if_pipeline_succeeds = var.require_pipeline
  remove_source_branch_after_merge      = true
  squash_option                         = "default_on"
}

resource "gitlab_branch_protection" "default" {
  project            = gitlab_project.this.id
  branch             = var.default_branch
  push_access_level  = "no one"
  merge_access_level = "maintainer"
  allow_force_push   = false
}

resource "gitlab_project_variable" "plain" {
  for_each = var.ci_variables

  project = gitlab_project.this.id
  key     = each.key
  value   = each.value
}

# The variables' names aren't secret, their values are: a resource per
# name, each value kept sensitive.
resource "gitlab_project_variable" "secret" {
  for_each = nonsensitive(toset(keys(var.ci_secrets)))

  project   = gitlab_project.this.id
  key       = each.key
  value     = var.ci_secrets[each.key]
  masked    = true
  protected = true
}

resource "gitlab_project_deploy_token" "this" {
  count   = length(var.deploy_token_scopes) > 0 ? 1 : 0
  project = gitlab_project.this.id
  name    = "${var.name}-deploy"
  scopes  = var.deploy_token_scopes
}
