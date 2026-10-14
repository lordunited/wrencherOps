variable "aws_region" {
  type        = string
  description = "The AWS region, e.g. eu-central-1."
}

variable "instances" {
  type = map(object({
    instance_type  = optional(string, "t3.small")
    ami_id         = optional(string, "")
    subnet_id      = optional(string, "")
    key_name       = optional(string, "")
    public_ip      = optional(bool, false)
    root_volume_gb = optional(number, 20)
    ingress = optional(list(object({
      port        = number
      cidr        = string
      protocol    = optional(string, "tcp")
      description = optional(string, "")
    })), [])
    user_data = optional(string, "")
    tags      = optional(map(string), {})
  }))
  description = "The instances to launch, by name. Edit them in instances.auto.tfvars."
}
