variable "kube_host" {
  type        = string
  description = "The API server's address, e.g. https://10.0.0.10:6443."
}

variable "kube_token" {
  type        = string
  description = "A service account token that may manage the apps' namespaces."
  sensitive   = true
}

variable "kube_ca_cert" {
  type        = string
  description = "The cluster's CA certificate (PEM), from the kubeconfig's certificate-authority-data, base64-decoded."
}

variable "apps" {
  type = map(object({
    namespace                 = string
    create_namespace          = optional(bool, false)
    image                     = string
    replicas                  = optional(number, 2)
    container_port            = optional(number, 8080)
    service_port              = optional(number, 80)
    service_type              = optional(string, "ClusterIP")
    env                       = optional(map(string), {})
    health_path               = optional(string, "/healthz")
    read_only_root_filesystem = optional(bool, true)
    cpu_request               = optional(string, "100m")
    memory_request            = optional(string, "128Mi")
    cpu_limit                 = optional(string, "500m")
    memory_limit              = optional(string, "256Mi")
  }))
  description = "The apps to run, by name. Edit them in apps.auto.tfvars."
}

variable "app_secrets" {
  type        = map(map(string))
  description = "Secret environment variables per app, e.g. { api = { DB_PASSWORD = \"...\" } }. Set it on the Variables tab, never in the code."
  sensitive   = true
  default     = {}
}
