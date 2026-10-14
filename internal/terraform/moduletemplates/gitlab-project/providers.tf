# A GitLab access token with the api scope, Owner or Maintainer in the
# groups. Set it on the Variables tab.
provider "gitlab" {
  base_url = var.gitlab_base_url
  token    = var.gitlab_token
}
