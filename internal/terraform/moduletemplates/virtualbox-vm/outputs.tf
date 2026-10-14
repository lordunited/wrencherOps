output "ip_addresses" {
  description = "Each VM's IPv4 address, once the guest reports one."
  value       = { for name, vm in module.vm : name => vm.ip_address }
}
