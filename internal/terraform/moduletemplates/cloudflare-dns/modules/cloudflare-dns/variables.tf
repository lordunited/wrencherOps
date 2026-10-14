variable "zone_name" {
  type        = string
  description = "The Cloudflare zone (domain) the records are in, e.g. example.com. It must exist already."
}

variable "records" {
  type = map(object({
    type     = string
    content  = string
    name     = optional(string)
    ttl      = optional(number, 1)
    proxied  = optional(bool, false)
    priority = optional(number)
    comment  = optional(string, "Managed by OpenTofu")
  }))
  description = "The records, by a unique key. name defaults to the key; use @ for the zone apex. ttl 1 is automatic."

  validation {
    condition     = alltrue([for r in var.records : contains(["A", "AAAA", "CNAME", "MX", "TXT", "SRV", "CAA", "NS"], r.type)])
    error_message = "Each record's type must be one of A, AAAA, CNAME, MX, TXT, SRV, CAA or NS."
  }
}
