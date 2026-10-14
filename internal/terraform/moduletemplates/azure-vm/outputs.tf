output "private_ips" {
  description = "Each VM's private IP."
  value       = { for name, vm in module.vm : name => vm.private_ip }
}

output "public_ips" {
  description = "Each VM's public IP, when it has one."
  value       = { for name, vm in module.vm : name => vm.public_ip }
}
