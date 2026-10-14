resource "helm_release" "this" {
  name             = var.name
  namespace        = var.namespace
  create_namespace = var.create_namespace
  repository       = var.repository
  chart            = var.chart
  version          = var.chart_version
  values           = var.values_yaml != "" ? [var.values_yaml] : []

  set_sensitive = [for path, value in var.secret_values : { name = path, value = value }]

  # A failed install or upgrade is rolled back, never left half done.
  atomic          = true
  cleanup_on_fail = true
  wait            = true
  timeout         = var.timeout
  max_history     = 10
}
