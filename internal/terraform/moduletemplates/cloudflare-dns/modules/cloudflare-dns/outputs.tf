output "zone_id" {
  description = "The zone's ID."
  value       = data.cloudflare_zone.this.zone_id
}

output "records" {
  description = "Each record's full name and type, by key."
  value       = { for k, r in cloudflare_dns_record.this : k => "${r.name} ${r.type}" }
}
