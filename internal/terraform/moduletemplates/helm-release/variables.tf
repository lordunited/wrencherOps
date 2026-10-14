variable "kube_host" {
  type        = string
  description = "The API server's address, e.g. https://10.0.0.10:6443."
}

variable "kube_token" {
  type        = string
  description = "A service account token that may install the releases."
  sensitive   = true
}

variable "kube_ca_cert" {
  type        = string
  description = "The cluster's CA certificate (PEM), from the kubeconfig's certificate-authority-data, base64-decoded."
}

variable "releases" {
  type = map(object({
    namespace        = string
    create_namespace = optional(bool, true)
    repository       = string
    chart            = string
    chart_version    = string
    values_yaml      = optional(string, "")
    timeout          = optional(number, 600)
  }))
  description = "The releases to install, by name. Edit them in releases.auto.tfvars."
}

variable "release_secrets" {
  type        = map(map(string))
  description = "Secret values per release, by dotted path, e.g. { redis = { \"auth.password\" = \"...\" } }. Set it on the Variables tab, never in the code."
  sensitive   = true
  default     = {}
}
