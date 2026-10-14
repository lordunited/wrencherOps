variable "vsphere_server" {
  type        = string
  description = "vCenter's address, e.g. vcenter.corp.local."
}

variable "vsphere_user" {
  type        = string
  description = "The vCenter user, e.g. administrator@vsphere.local."
}

variable "vsphere_password" {
  type        = string
  description = "The vCenter user's password."
  sensitive   = true
}

variable "vsphere_allow_unverified_ssl" {
  type        = bool
  description = "Accept vCenter's self-signed certificate. Leave false when it has a trusted one."
  default     = false
}

variable "vms" {
  type = map(object({
    datacenter   = string
    cluster      = string
    datastore    = string
    network      = string
    template     = string
    folder       = optional(string, "")
    num_cpus     = optional(number, 2)
    memory       = optional(number, 4096)
    disk_size    = optional(number, 40)
    linked_clone = optional(bool, false)
    domain       = optional(string, "local")
    ipv4_address = optional(string, "")
    ipv4_netmask = optional(number, 24)
    ipv4_gateway = optional(string, "")
    dns_servers  = optional(list(string), [])
    annotation   = optional(string, "Managed by OpenTofu")
  }))
  description = "The VMs to create, by name. Edit them in vms.auto.tfvars."
}
