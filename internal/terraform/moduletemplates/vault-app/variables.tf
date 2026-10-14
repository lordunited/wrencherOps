variable "vault_address" {
  type        = string
  description = "Vault's address, e.g. https://vault.corp.local:8200."
}

variable "vault_token" {
  type        = string
  description = "A Vault token that may manage mounts, policies and auth methods."
  sensitive   = true
}

variable "vault_skip_tls_verify" {
  type        = bool
  description = "Accept Vault's self-signed certificate. Leave false when it has a trusted one."
  default     = false
}

variable "kv_mount_path" {
  type        = string
  description = "The KV v2 mount the apps' secrets go in."
  default     = "apps"
}

variable "create_kv_mount" {
  type        = bool
  description = "Create the KV v2 mount. false when it exists already."
  default     = true
}

variable "approle_path" {
  type        = string
  description = "The path AppRole auth is enabled at."
  default     = "approle"
}

variable "create_approle_backend" {
  type        = bool
  description = "Enable AppRole auth at approle_path. false when it's enabled already."
  default     = true
}

variable "apps" {
  type = map(object({
    token_ttl      = optional(number, 3600)
    token_max_ttl  = optional(number, 14400)
    secret_id_ttl  = optional(number, 0)
    bound_cidrs    = optional(list(string), [])
    extra_policies = optional(list(string), [])
  }))
  description = "The apps, by name. Edit them in apps.auto.tfvars."
}

variable "app_secrets" {
  type        = map(map(string))
  description = "Each app's secret values, by app name, e.g. { billing = { db_password = \"...\" } }. Set it on the Variables tab, never in the code."
  sensitive   = true
  default     = {}
}
