output "private_ips" {
  description = "Each instance's private IP."
  value       = { for name, i in module.instance : name => i.private_ip }
}

output "public_ips" {
  description = "Each instance's public IP, when it has one."
  value       = { for name, i in module.instance : name => i.public_ip }
}
