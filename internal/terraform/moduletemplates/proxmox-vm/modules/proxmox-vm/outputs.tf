output "vm_id" {
  description = "The VM's ID in Proxmox."
  value       = proxmox_virtual_environment_vm.this.vm_id
}

output "ip_addresses" {
  description = "The VM's IPv4 addresses, as the guest agent reports them (loopback left out)."
  value       = [for ip in flatten(proxmox_virtual_environment_vm.this.ipv4_addresses) : ip if ip != "127.0.0.1"]
}
