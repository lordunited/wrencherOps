# The apps this workspace runs: rename, add or remove entries, then plan.
# Secret env vars go in app_secrets on the Variables tab. Only one app per
# namespace should set create_namespace = true.
apps = {
  "api" = {
    namespace        = "shop"
    create_namespace = true
    image            = "ghcr.io/acme/api:1.4.2"
    replicas         = 2
    container_port   = 8080
    env              = { LOG_LEVEL = "info" }
  }
}
