# One Deployment and Service per entry of var.apps (see apps.auto.tfvars),
# each from the reusable module in modules/kubernetes-app.
module "app" {
  source   = "./modules/kubernetes-app"
  for_each = var.apps

  name             = each.key
  namespace        = each.value.namespace
  create_namespace = each.value.create_namespace
  image            = each.value.image
  replicas         = each.value.replicas
  container_port   = each.value.container_port
  service_port     = each.value.service_port
  service_type     = each.value.service_type
  env              = merge(each.value.env, lookup(var.app_secrets, each.key, {}))
  health_path      = each.value.health_path
  cpu_request      = each.value.cpu_request
  memory_request   = each.value.memory_request
  cpu_limit        = each.value.cpu_limit
  memory_limit     = each.value.memory_limit

  read_only_root_filesystem = each.value.read_only_root_filesystem
}
