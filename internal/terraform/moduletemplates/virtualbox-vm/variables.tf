variable "vms" {
  type = map(object({
    image          = string
    cpus           = optional(number, 2)
    memory_mb      = optional(number, 2048)
    network_type   = optional(string, "hostonly")
    host_interface = optional(string, "vboxnet0")
    user_data      = optional(string, "")
  }))
  description = "The VMs to create, by name. Edit them in vms.auto.tfvars."
}
