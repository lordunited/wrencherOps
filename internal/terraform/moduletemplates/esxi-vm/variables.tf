variable "esxi_hostname" {
  type        = string
  description = "The ESXi host's address."
}

variable "esxi_username" {
  type        = string
  description = "The ESXi user to log in as."
  default     = "root"
}

variable "esxi_password" {
  type        = string
  description = "The ESXi user's password."
  sensitive   = true
}

variable "esxi_ssh_port" {
  type        = number
  description = "The host's SSH port."
  default     = 22
}

variable "esxi_https_port" {
  type        = number
  description = "The host's HTTPS port."
  default     = 443
}

variable "vms" {
  type = map(object({
    disk_store     = string
    clone_from_vm  = optional(string, "")
    guestos        = optional(string, "ubuntu-64")
    numvcpus       = optional(number, 2)
    memsize        = optional(number, 2048)
    boot_disk_size = optional(number, 20)
    boot_disk_type = optional(string, "thin")
    networks       = optional(list(string), ["VM Network"])
    power          = optional(string, "on")
    notes          = optional(string, "Managed by OpenTofu")
  }))
  description = "The VMs to create, by name. Edit them in vms.auto.tfvars."
}
