data "cloudflare_zone" "this" {
  filter = {
    name = var.zone_name
  }
}

resource "cloudflare_dns_record" "this" {
  for_each = var.records

  zone_id  = data.cloudflare_zone.this.zone_id
  name     = coalesce(each.value.name, each.key) == "@" ? var.zone_name : coalesce(each.value.name, each.key)
  type     = each.value.type
  content  = each.value.content
  ttl      = each.value.proxied ? 1 : each.value.ttl
  proxied  = contains(["A", "AAAA", "CNAME"], each.value.type) ? each.value.proxied : null
  priority = each.value.type == "MX" ? coalesce(each.value.priority, 10) : each.value.priority
  comment  = each.value.comment
}
