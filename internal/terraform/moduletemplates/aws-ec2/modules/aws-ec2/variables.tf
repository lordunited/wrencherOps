variable "name" {
  type        = string
  description = "The instance's Name tag, also its security group's name."
}

variable "instance_type" {
  type        = string
  description = "The instance type, e.g. t3.small."
  default     = "t3.small"
}

variable "ami_id" {
  type        = string
  description = "The AMI to boot. Empty for the latest Ubuntu 24.04 LTS from Canonical."
  default     = ""
}

variable "subnet_id" {
  type        = string
  description = "The subnet to launch in. Empty for the first subnet of the default VPC."
  default     = ""
}

variable "key_name" {
  type        = string
  description = "An EC2 key pair to log in with over SSH. Empty for none (use SSM Session Manager)."
  default     = ""
}

variable "public_ip" {
  type        = bool
  description = "Give the instance a public IP."
  default     = false
}

variable "root_volume_gb" {
  type        = number
  description = "Root volume size in GB (gp3, encrypted)."
  default     = 20
}

variable "ingress" {
  type = list(object({
    port        = number
    cidr        = string
    protocol    = optional(string, "tcp")
    description = optional(string, "")
  }))
  description = "Ports to open, each from one IPv4 range, e.g. [{ port = 22, cidr = \"203.0.113.0/24\" }]."
  default     = []
}

variable "user_data" {
  type        = string
  description = "A script or cloud-init config run on first boot. Empty for none."
  default     = ""
}

variable "tags" {
  type        = map(string)
  description = "Extra tags on the instance and its security group."
  default     = {}
}
