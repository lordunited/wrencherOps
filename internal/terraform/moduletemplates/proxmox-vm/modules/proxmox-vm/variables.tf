variable "name" {
  type        = string
  description = "The VM's name, also its cloud-init host name."
}

variable "node_name" {
  type        = string
  description = "The Proxmox node the VM runs on, e.g. pve."
}

variable "template_vm_id" {
  type        = number
  description = "The ID of the template VM to clone (with cloud-init and the QEMU guest agent installed)."
}

variable "vm_id" {
  type        = number
  description = "The new VM's ID. null picks the next free one."
  default     = null
}

variable "datastore_id" {
  type        = string
  description = "The storage for the VM's disk and cloud-init drive, e.g. local-lvm."
  default     = "local-lvm"
}

variable "cores" {
  type        = number
  description = "CPU cores."
  default     = 2
}

variable "memory_mb" {
  type        = number
  description = "Memory in MB."
  default     = 2048
}

variable "disk_size" {
  type        = number
  description = "Boot disk size in GB. It can't be smaller than the template's."
  default     = 20
}

variable "bridge" {
  type        = string
  description = "The network bridge, e.g. vmbr0."
  default     = "vmbr0"
}

variable "vlan_id" {
  type        = number
  description = "A VLAN tag for the network card. null for none."
  default     = null
}

variable "ipv4_address" {
  type        = string
  description = "A static address with its prefix, e.g. 10.0.20.31/24, or dhcp."
  default     = "dhcp"
}

variable "ipv4_gateway" {
  type        = string
  description = "The default gateway, with a static address."
  default     = ""
}

variable "username" {
  type        = string
  description = "The user cloud-init creates."
  default     = "ubuntu"
}

variable "ssh_public_keys" {
  type        = list(string)
  description = "SSH public keys that may log in as the user."
  default     = []
}

variable "tags" {
  type        = list(string)
  description = "Tags shown on the VM in Proxmox."
  default     = ["opentofu"]
}
