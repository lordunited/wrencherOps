variable "name" {
  type        = string
  description = "The VM's name in vCenter, also its host name."
}

variable "datacenter" {
  type        = string
  description = "The datacenter, e.g. DC1."
}

variable "cluster" {
  type        = string
  description = "The compute cluster whose root resource pool the VM runs in."
}

variable "datastore" {
  type        = string
  description = "The datastore for the VM's disks."
}

variable "network" {
  type        = string
  description = "The port group or distributed port group, e.g. VM Network."
}

variable "template" {
  type        = string
  description = "The template VM to clone, e.g. ubuntu-24.04-template."
}

variable "folder" {
  type        = string
  description = "The VM folder, relative to the datacenter. Empty for the root."
  default     = ""
}

variable "num_cpus" {
  type        = number
  description = "Virtual CPUs."
  default     = 2
}

variable "memory" {
  type        = number
  description = "Memory in MB."
  default     = 4096
}

variable "disk_size" {
  type        = number
  description = "Boot disk size in GB. It can't be smaller than the template's."
  default     = 40
}

variable "linked_clone" {
  type        = bool
  description = "Clone from the template's snapshot: faster, but the VM depends on the template."
  default     = false
}

variable "domain" {
  type        = string
  description = "The guest's DNS domain, e.g. corp.local."
  default     = "local"
}

variable "ipv4_address" {
  type        = string
  description = "A static IPv4 address. Empty for DHCP."
  default     = ""
}

variable "ipv4_netmask" {
  type        = number
  description = "The static address's prefix length, e.g. 24."
  default     = 24
}

variable "ipv4_gateway" {
  type        = string
  description = "The default gateway, with a static address."
  default     = ""
}

variable "dns_servers" {
  type        = list(string)
  description = "DNS servers, with a static address."
  default     = []
}

variable "annotation" {
  type        = string
  description = "Notes shown on the VM in vCenter."
  default     = "Managed by OpenTofu"
}
