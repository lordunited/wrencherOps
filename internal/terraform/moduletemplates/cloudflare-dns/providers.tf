# An API token with Zone › DNS › Edit (and Zone › Zone › Read) on the zones.
# Set it on the Variables tab.
provider "cloudflare" {
  api_token = var.cloudflare_api_token
}
