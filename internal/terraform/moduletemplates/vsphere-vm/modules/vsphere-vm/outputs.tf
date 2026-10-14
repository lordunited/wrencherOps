output "id" {
  description = "The VM's UUID."
  value       = vsphere_virtual_machine.this.id
}

output "ip_address" {
  description = "The VM's main IP, as VMware Tools reports it."
  value       = vsphere_virtual_machine.this.default_ip_address
}
