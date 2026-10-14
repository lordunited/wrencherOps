# Vault's address and a token that may manage mounts, policies and auth
# methods. Set them on the Variables tab.
provider "vault" {
  address         = var.vault_address
  token           = var.vault_token
  skip_tls_verify = var.vault_skip_tls_verify
}
