output "id" {
  description = "The instance's ID."
  value       = aws_instance.this.id
}

output "private_ip" {
  description = "The instance's private IP."
  value       = aws_instance.this.private_ip
}

output "public_ip" {
  description = "The instance's public IP, when it has one."
  value       = aws_instance.this.public_ip
}

output "security_group_id" {
  description = "The instance's security group."
  value       = aws_security_group.this.id
}
