variable "name" {
  type        = string
  description = "The instance's name (lowercase letters, digits and dashes)."

  validation {
    condition     = can(regex("^[a-z]([-a-z0-9]{0,61}[a-z0-9])?$", var.name))
    error_message = "name must be lowercase letters, digits and dashes, starting with a letter."
  }
}

variable "zone" {
  type        = string
  description = "The zone, e.g. europe-west3-a."
}

variable "machine_type" {
  type        = string
  description = "The machine type, e.g. e2-small."
  default     = "e2-small"
}

variable "image" {
  type        = string
  description = "The boot image, e.g. ubuntu-os-cloud/ubuntu-2404-lts-amd64."
  default     = "ubuntu-os-cloud/ubuntu-2404-lts-amd64"
}

variable "disk_size_gb" {
  type        = number
  description = "Boot disk size in GB (balanced persistent disk)."
  default     = 20
}

variable "network" {
  type        = string
  description = "The VPC network."
  default     = "default"
}

variable "subnetwork" {
  type        = string
  description = "The subnetwork. Empty for the network's subnet in the zone's region (auto mode networks)."
  default     = ""
}

variable "public_ip" {
  type        = bool
  description = "Give the instance an ephemeral public IP."
  default     = false
}

variable "ingress" {
  type = list(object({
    port   = number
    source = string
  }))
  description = "TCP ports to open to this instance, each from one IPv4 range."
  default     = []
}

variable "ssh_user" {
  type        = string
  description = "The user the SSH key logs in as."
  default     = "ubuntu"
}

variable "ssh_public_key" {
  type        = string
  description = "An SSH public key that may log in. Empty for none (use OS Login or IAP)."
  default     = ""
}

variable "startup_script" {
  type        = string
  description = "A script run on every boot. Empty for none."
  default     = ""
}

variable "labels" {
  type        = map(string)
  description = "Labels on the instance (lowercase keys and values)."
  default     = {}
}
