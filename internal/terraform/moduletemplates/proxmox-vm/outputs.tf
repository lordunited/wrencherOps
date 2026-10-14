output "ip_addresses" {
  description = "Each VM's IPv4 addresses, as the guest agent reports them."
  value       = { for name, vm in module.vm : name => vm.ip_addresses }
}
