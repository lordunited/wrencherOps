output "id" {
  description = "The VM's ID on the ESXi host."
  value       = esxi_guest.this.id
}

output "ip_address" {
  description = "The IP VMware Tools reports, empty until the guest has one."
  value       = esxi_guest.this.ip_address
}
