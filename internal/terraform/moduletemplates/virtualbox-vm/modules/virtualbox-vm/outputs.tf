output "id" {
  description = "The VM's UUID in VirtualBox."
  value       = virtualbox_vm.this.id
}

output "ip_address" {
  description = "The IPv4 address of the VM's network card, once the guest reports one."
  value       = virtualbox_vm.this.network_adapter[0].ipv4_address
}
