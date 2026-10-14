# The Helm releases this workspace installs: rename, add or remove entries,
# then plan. Secret values go in release_secrets on the Variables tab.
releases = {
  "ingress-nginx" = {
    namespace     = "ingress-nginx"
    repository    = "https://kubernetes.github.io/ingress-nginx"
    chart         = "ingress-nginx"
    chart_version = "4.11.3"
    values_yaml   = <<-EOT
      controller:
        replicaCount: 2
        service:
          type: LoadBalancer
    EOT
  }
}
