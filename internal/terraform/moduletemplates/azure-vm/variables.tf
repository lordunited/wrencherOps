variable "azure_subscription_id" {
  type        = string
  description = "The subscription the resources go in."
}

variable "resource_group_name" {
  type        = string
  description = "The resource group this workspace creates for its VMs and network."
  default     = "rg-opentofu-vms"
}

variable "location" {
  type        = string
  description = "The Azure region, e.g. westeurope."
  default     = "westeurope"
}

variable "address_space" {
  type        = string
  description = "The virtual network's address range."
  default     = "10.40.0.0/16"
}

variable "subnet_prefix" {
  type        = string
  description = "The VMs' subnet, inside address_space."
  default     = "10.40.1.0/24"
}

variable "tags" {
  type        = map(string)
  description = "Tags on everything this workspace creates."
  default     = { ManagedBy = "OpenTofu" }
}

variable "vms" {
  type = map(object({
    ssh_public_key = string
    size           = optional(string, "Standard_B2s")
    admin_username = optional(string, "azureuser")
    public_ip      = optional(bool, true)
    os_disk_gb     = optional(number, 30)
    ingress = optional(list(object({
      port   = number
      source = string
    })), [])
    custom_data = optional(string, "")
  }))
  description = "The VMs to create, by name. Edit them in vms.auto.tfvars."
}
