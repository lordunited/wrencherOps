# The apps this workspace gives a secret and an AppRole login: rename, add
# or remove entries, then plan. The secret values themselves go in the
# app_secrets variable on the Variables tab, never here.
apps = {
  "billing" = {
    token_ttl   = 3600
    bound_cidrs = ["10.0.0.0/16"]
  }
}
