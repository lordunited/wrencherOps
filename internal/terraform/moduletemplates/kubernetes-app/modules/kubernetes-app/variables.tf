variable "name" {
  type        = string
  description = "The app's name: its Deployment, Service and labels."
}

variable "namespace" {
  type        = string
  description = "The namespace the app runs in."
}

variable "create_namespace" {
  type        = bool
  description = "Create the namespace. false when it exists already."
  default     = false
}

variable "image" {
  type        = string
  description = "The image with a pinned tag, e.g. ghcr.io/acme/api:1.4.2."

  validation {
    condition     = can(regex(":[^/]+$|@sha256:", var.image)) && !endswith(var.image, ":latest")
    error_message = "Pin the image to a tag or digest (not :latest), so every apply runs the same thing."
  }
}

variable "replicas" {
  type        = number
  description = "How many pods run."
  default     = 2
}

variable "container_port" {
  type        = number
  description = "The port the container listens on."
  default     = 8080
}

variable "service_port" {
  type        = number
  description = "The port the Service exposes."
  default     = 80
}

variable "service_type" {
  type        = string
  description = "ClusterIP, NodePort or LoadBalancer."
  default     = "ClusterIP"

  validation {
    condition     = contains(["ClusterIP", "NodePort", "LoadBalancer"], var.service_type)
    error_message = "service_type must be ClusterIP, NodePort or LoadBalancer."
  }
}

variable "env" {
  type        = map(string)
  description = "Environment variables. Pass secrets through a sensitive variable, not in the code."
  default     = {}
}

variable "health_path" {
  type        = string
  description = "The HTTP path the readiness and liveness probes check. Empty for no probes."
  default     = "/healthz"
}

variable "read_only_root_filesystem" {
  type        = bool
  description = "Mount the container's filesystem read-only. false for images that write outside their volumes, e.g. to /tmp."
  default     = true
}

variable "cpu_request" {
  type        = string
  description = "CPU each pod is guaranteed, e.g. 100m."
  default     = "100m"
}

variable "memory_request" {
  type        = string
  description = "Memory each pod is guaranteed, e.g. 128Mi."
  default     = "128Mi"
}

variable "cpu_limit" {
  type        = string
  description = "The most CPU a pod may use, e.g. 500m."
  default     = "500m"
}

variable "memory_limit" {
  type        = string
  description = "The most memory a pod may use, e.g. 256Mi."
  default     = "256Mi"
}
