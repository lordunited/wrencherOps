# The projects this workspace manages: rename, add or remove entries, then
# plan. Secret CI/CD variables go in project_ci_secrets on the Variables tab.
projects = {
  "billing-api" = {
    group_path          = "platform/services"
    description         = "Billing service API"
    ci_variables        = { DEPLOY_ENV = "staging" }
    deploy_token_scopes = ["read_registry"]
  }
}
