# The cluster's API server and a service account token that may manage the
# apps' namespaces. Set them on the Variables tab. A token for a service
# account "tofu": kubectl create token tofu -n kube-system --duration=8760h
provider "kubernetes" {
  host                   = var.kube_host
  token                  = var.kube_token
  cluster_ca_certificate = var.kube_ca_cert
}
