variable "name" {
  type        = string
  description = "The container's name."
}

variable "image" {
  type        = string
  description = "The image with a pinned tag, e.g. nginx:1.27-alpine."

  validation {
    condition     = can(regex(":[^/]+$|@sha256:", var.image)) && !endswith(var.image, ":latest")
    error_message = "Pin the image to a tag or digest (not :latest), so every apply runs the same thing."
  }
}

variable "ports" {
  type = list(object({
    internal = number
    external = number
    protocol = optional(string, "tcp")
    ip       = optional(string, "0.0.0.0")
  }))
  description = "Ports to publish on the host."
  default     = []
}

variable "env" {
  type        = map(string)
  description = "Environment variables. Pass secrets through a sensitive variable, not in the code."
  default     = {}
}

variable "volumes" {
  type = list(object({
    container_path = string
    host_path      = optional(string)
    volume_name    = optional(string)
    read_only      = optional(bool, false)
  }))
  description = "Host folders or named volumes to mount: give host_path or volume_name."
  default     = []
}

variable "command" {
  type        = list(string)
  description = "Overrides the image's command. Empty keeps it."
  default     = []
}

variable "restart" {
  type        = string
  description = "Restart policy: no, on-failure, always or unless-stopped."
  default     = "unless-stopped"

  validation {
    condition     = contains(["no", "on-failure", "always", "unless-stopped"], var.restart)
    error_message = "restart must be no, on-failure, always or unless-stopped."
  }
}

variable "memory_mb" {
  type        = number
  description = "Memory limit in MB. 0 for none."
  default     = 512
}

variable "cpus" {
  type        = string
  description = "CPU limit, e.g. \"1.5\". Empty for none."
  default     = "1"
}

variable "healthcheck" {
  type        = list(string)
  description = "A health check command, e.g. [\"CMD\", \"wget\", \"-qO-\", \"http://localhost/\"]. Empty for the image's own."
  default     = []
}

variable "labels" {
  type        = map(string)
  description = "Container labels."
  default     = {}
}
