variable "name" {
  type        = string
  description = "The VM's name on the ESXi host."
}

variable "disk_store" {
  type        = string
  description = "The datastore the VM's files go on, e.g. datastore1."
}

variable "clone_from_vm" {
  type        = string
  description = "A powered-off VM on the same host to copy, e.g. ubuntu-template. Empty for a new, empty VM."
  default     = ""
}

variable "guestos" {
  type        = string
  description = "The guest OS type of an empty VM (ignored when cloning), e.g. ubuntu-64 or windows9srv-64."
  default     = "ubuntu-64"
}

variable "numvcpus" {
  type        = number
  description = "Virtual CPUs."
  default     = 2

  validation {
    condition     = var.numvcpus >= 1 && floor(var.numvcpus) == var.numvcpus
    error_message = "numvcpus must be a whole number, at least 1."
  }
}

variable "memsize" {
  type        = number
  description = "Memory in MB."
  default     = 2048

  validation {
    condition     = var.memsize >= 128
    error_message = "memsize must be at least 128 MB."
  }
}

variable "boot_disk_size" {
  type        = number
  description = "Boot disk size in GB. A clone keeps its template's disk if this is smaller."
  default     = 20
}

variable "boot_disk_type" {
  type        = string
  description = "How the boot disk is provisioned: thin, zeroedthick or eagerzeroedthick."
  default     = "thin"

  validation {
    condition     = contains(["thin", "zeroedthick", "eagerzeroedthick"], var.boot_disk_type)
    error_message = "boot_disk_type must be thin, zeroedthick or eagerzeroedthick."
  }
}

variable "networks" {
  type        = list(string)
  description = "Port groups to connect, one network card each, e.g. [\"VM Network\"]."
  default     = ["VM Network"]
}

variable "power" {
  type        = string
  description = "on or off after creation."
  default     = "on"

  validation {
    condition     = contains(["on", "off"], var.power)
    error_message = "power must be on or off."
  }
}

variable "notes" {
  type        = string
  description = "Notes shown on the VM in ESXi."
  default     = "Managed by OpenTofu"
}
