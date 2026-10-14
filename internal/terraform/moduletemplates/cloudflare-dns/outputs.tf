output "records" {
  description = "Each zone's records."
  value       = { for zone, z in module.zone : zone => z.records }
}
