variable "proxmox_endpoint" {
  type        = string
  description = "Proxmox's API address, e.g. https://pve.corp.local:8006/."
}

variable "proxmox_api_token" {
  type        = string
  description = "An API token, as user@realm!tokenid=secret."
  sensitive   = true
}

variable "proxmox_insecure" {
  type        = bool
  description = "Accept Proxmox's self-signed certificate. Leave false when it has a trusted one."
  default     = false
}

variable "vms" {
  type = map(object({
    node_name       = string
    template_vm_id  = number
    vm_id           = optional(number)
    datastore_id    = optional(string, "local-lvm")
    cores           = optional(number, 2)
    memory_mb       = optional(number, 2048)
    disk_size       = optional(number, 20)
    bridge          = optional(string, "vmbr0")
    vlan_id         = optional(number)
    ipv4_address    = optional(string, "dhcp")
    ipv4_gateway    = optional(string, "")
    username        = optional(string, "ubuntu")
    ssh_public_keys = optional(list(string), [])
    tags            = optional(list(string), ["opentofu"])
  }))
  description = "The VMs to create, by name. Edit them in vms.auto.tfvars."
}
