variable "name" {
  type        = string
  description = "The VM's name in VirtualBox."
}

variable "image" {
  type        = string
  description = "A Vagrant box (.box) to build the VM from: a URL or a local path."
}

variable "cpus" {
  type        = number
  description = "Virtual CPUs."
  default     = 2
}

variable "memory_mb" {
  type        = number
  description = "Memory in MB."
  default     = 2048
}

variable "network_type" {
  type        = string
  description = "The network card's mode: nat, bridged or hostonly."
  default     = "hostonly"

  validation {
    condition     = contains(["nat", "bridged", "hostonly"], var.network_type)
    error_message = "network_type must be nat, bridged or hostonly."
  }
}

variable "host_interface" {
  type        = string
  description = "The host's interface for bridged or hostonly, e.g. vboxnet0 or en0. Empty for nat."
  default     = "vboxnet0"
}

variable "user_data" {
  type        = string
  description = "cloud-init user data for images that support it. Empty for none."
  default     = ""
}
