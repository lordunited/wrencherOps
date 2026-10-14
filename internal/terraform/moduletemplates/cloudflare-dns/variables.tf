variable "cloudflare_api_token" {
  type        = string
  description = "A Cloudflare API token that may edit DNS in the zones."
  sensitive   = true
}

variable "zones" {
  type = map(object({
    records = map(object({
      type     = string
      content  = string
      name     = optional(string)
      ttl      = optional(number, 1)
      proxied  = optional(bool, false)
      priority = optional(number)
      comment  = optional(string, "Managed by OpenTofu")
    }))
  }))
  description = "The zones and their records, by zone name. Edit them in dns.auto.tfvars."
}
