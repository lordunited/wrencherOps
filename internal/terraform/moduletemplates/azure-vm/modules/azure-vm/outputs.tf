output "id" {
  description = "The VM's resource ID."
  value       = azurerm_linux_virtual_machine.this.id
}

output "private_ip" {
  description = "The VM's private IP."
  value       = azurerm_linux_virtual_machine.this.private_ip_address
}

output "public_ip" {
  description = "The VM's public IP, when it has one."
  value       = var.public_ip ? azurerm_public_ip.this[0].ip_address : null
}
