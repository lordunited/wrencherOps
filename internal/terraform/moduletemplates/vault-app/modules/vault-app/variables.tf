variable "name" {
  type        = string
  description = "The app's name: its secret's path, its policy and its AppRole role."
}

variable "kv_mount" {
  type        = string
  description = "The KV v2 mount the app's secret is kept in, e.g. apps."
}

variable "approle_backend" {
  type        = string
  description = "The path AppRole auth is enabled at, e.g. approle."
}

variable "secrets" {
  type        = map(string)
  description = "The app's secret values, written to <kv_mount>/<name>."
  sensitive   = true
  default     = {}
}

variable "token_ttl" {
  type        = number
  description = "How long the app's Vault token lasts, in seconds."
  default     = 3600
}

variable "token_max_ttl" {
  type        = number
  description = "The longest the app's token can be renewed to, in seconds."
  default     = 14400
}

variable "secret_id_ttl" {
  type        = number
  description = "How long a SecretID lasts, in seconds. 0 never expires."
  default     = 0
}

variable "bound_cidrs" {
  type        = list(string)
  description = "Networks the app may log in from, e.g. [\"10.0.0.0/16\"]. Empty for anywhere."
  default     = []
}

variable "extra_policies" {
  type        = list(string)
  description = "Other Vault policies to give the app's token."
  default     = []
}
