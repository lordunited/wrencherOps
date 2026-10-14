variable "gcp_project" {
  type        = string
  description = "The project ID, e.g. acme-prod-123456."
}

variable "gcp_region" {
  type        = string
  description = "The default region, e.g. europe-west3."
  default     = "europe-west3"
}

variable "vms" {
  type = map(object({
    zone         = string
    machine_type = optional(string, "e2-small")
    image        = optional(string, "ubuntu-os-cloud/ubuntu-2404-lts-amd64")
    disk_size_gb = optional(number, 20)
    network      = optional(string, "default")
    subnetwork   = optional(string, "")
    public_ip    = optional(bool, false)
    ingress = optional(list(object({
      port   = number
      source = string
    })), [])
    ssh_user       = optional(string, "ubuntu")
    ssh_public_key = optional(string, "")
    startup_script = optional(string, "")
    labels         = optional(map(string), {})
  }))
  description = "The instances to create, by name. Edit them in vms.auto.tfvars."
}
