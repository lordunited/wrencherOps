variable "docker_host" {
  type        = string
  description = "The Docker host, e.g. tcp://10.0.0.5:2376, ssh://deploy@10.0.0.5 or unix:///var/run/docker.sock."
}

variable "docker_ca_cert" {
  type        = string
  description = "The CA certificate (PEM) of a TLS Docker host. Empty without TLS."
  default     = ""
}

variable "docker_client_cert" {
  type        = string
  description = "The client certificate (PEM) for a TLS Docker host. Empty without TLS."
  default     = ""
}

variable "docker_client_private_key" {
  type        = string
  description = "The client certificate's private key (PEM). A secret, so it has no default: set it to an empty value on the Variables tab when the host doesn't use TLS."
  sensitive   = true
}

variable "containers" {
  type = map(object({
    image = string
    ports = optional(list(object({
      internal = number
      external = number
      protocol = optional(string, "tcp")
      ip       = optional(string, "0.0.0.0")
    })), [])
    env = optional(map(string), {})
    volumes = optional(list(object({
      container_path = string
      host_path      = optional(string)
      volume_name    = optional(string)
      read_only      = optional(bool, false)
    })), [])
    command     = optional(list(string), [])
    restart     = optional(string, "unless-stopped")
    memory_mb   = optional(number, 512)
    cpus        = optional(string, "1")
    healthcheck = optional(list(string), [])
    labels      = optional(map(string), {})
  }))
  description = "The containers to run, by name. Edit them in containers.auto.tfvars."
}

variable "container_secrets" {
  type        = map(map(string))
  description = "Secret environment variables per container, e.g. { web = { DB_PASSWORD = \"...\" } }. Set it on the Variables tab, never in the code."
  sensitive   = true
  default     = {}
}
