# One Helm release per entry of var.releases (see releases.auto.tfvars),
# each from the reusable module in modules/helm-release.
module "release" {
  source   = "./modules/helm-release"
  for_each = var.releases

  name             = each.key
  namespace        = each.value.namespace
  create_namespace = each.value.create_namespace
  repository       = each.value.repository
  chart            = each.value.chart
  chart_version    = each.value.chart_version
  values_yaml      = each.value.values_yaml
  secret_values    = lookup(var.release_secrets, each.key, {})
  timeout          = each.value.timeout
}
