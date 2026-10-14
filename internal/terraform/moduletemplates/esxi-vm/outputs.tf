output "ip_addresses" {
  description = "Each VM's IP, as VMware Tools reports it."
  value       = { for name, vm in module.vm : name => vm.ip_address }
}
