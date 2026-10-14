# One container per entry of var.containers (see containers.auto.tfvars),
# each from the reusable module in modules/docker-container.
module "container" {
  source   = "./modules/docker-container"
  for_each = var.containers

  name        = each.key
  image       = each.value.image
  ports       = each.value.ports
  env         = merge(each.value.env, lookup(var.container_secrets, each.key, {}))
  volumes     = each.value.volumes
  command     = each.value.command
  restart     = each.value.restart
  memory_mb   = each.value.memory_mb
  cpus        = each.value.cpus
  healthcheck = each.value.healthcheck
  labels      = each.value.labels
}
