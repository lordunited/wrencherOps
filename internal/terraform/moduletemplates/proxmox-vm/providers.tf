# Proxmox's API, with an API token (Datacenter › Permissions › API Tokens):
# set proxmox_api_token on the Variables tab as "user@realm!tokenid=secret".
provider "proxmox" {
  endpoint  = var.proxmox_endpoint
  api_token = var.proxmox_api_token
  insecure  = var.proxmox_insecure
}
