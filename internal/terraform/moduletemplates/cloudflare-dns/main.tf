# The records of each zone in var.zones (see dns.auto.tfvars), each zone
# from the reusable module in modules/cloudflare-dns.
module "zone" {
  source   = "./modules/cloudflare-dns"
  for_each = var.zones

  zone_name = each.key
  records   = each.value.records
}
