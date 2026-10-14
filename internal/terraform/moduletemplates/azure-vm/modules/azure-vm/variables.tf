variable "name" {
  type        = string
  description = "The VM's name, also the prefix of its network resources."
}

variable "resource_group_name" {
  type        = string
  description = "The resource group the VM and its network resources go in."
}

variable "location" {
  type        = string
  description = "The Azure region, e.g. westeurope."
}

variable "subnet_id" {
  type        = string
  description = "The subnet the VM's network card is in."
}

variable "size" {
  type        = string
  description = "The VM size, e.g. Standard_B2s."
  default     = "Standard_B2s"
}

variable "admin_username" {
  type        = string
  description = "The Linux user to log in as."
  default     = "azureuser"
}

variable "ssh_public_key" {
  type        = string
  description = "The SSH public key that may log in; password login is off."
}

variable "public_ip" {
  type        = bool
  description = "Give the VM a static public IP."
  default     = true
}

variable "os_disk_gb" {
  type        = number
  description = "OS disk size in GB."
  default     = 30
}

variable "ingress" {
  type = list(object({
    port   = number
    source = string
  }))
  description = "TCP ports to open, each from one address range or service tag, e.g. [{ port = 22, source = \"203.0.113.0/24\" }]."
  default     = []
}

variable "custom_data" {
  type        = string
  description = "cloud-init config run on first boot. Empty for none."
  default     = ""
}

variable "tags" {
  type        = map(string)
  description = "Tags on the VM and its network resources."
  default     = {}
}
