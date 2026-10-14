# One project per entry of var.projects (see projects.auto.tfvars), each
# from the reusable module in modules/gitlab-project.
module "project" {
  source   = "./modules/gitlab-project"
  for_each = var.projects

  name                = each.key
  group_path          = each.value.group_path
  description         = each.value.description
  visibility          = each.value.visibility
  default_branch      = each.value.default_branch
  require_pipeline    = each.value.require_pipeline
  ci_variables        = each.value.ci_variables
  ci_secrets          = lookup(var.project_ci_secrets, each.key, {})
  deploy_token_scopes = each.value.deploy_token_scopes
}
