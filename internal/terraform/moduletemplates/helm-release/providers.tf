# The cluster's API server and a service account token that may install
# the releases. Set them on the Variables tab. A token for a service
# account "tofu": kubectl create token tofu -n kube-system --duration=8760h
provider "helm" {
  kubernetes = {
    host                   = var.kube_host
    token                  = var.kube_token
    cluster_ca_certificate = var.kube_ca_cert
  }
}
