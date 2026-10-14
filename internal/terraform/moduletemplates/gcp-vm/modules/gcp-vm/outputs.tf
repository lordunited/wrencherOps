output "id" {
  description = "The instance's ID."
  value       = google_compute_instance.this.instance_id
}

output "private_ip" {
  description = "The instance's internal IP."
  value       = google_compute_instance.this.network_interface[0].network_ip
}

output "public_ip" {
  description = "The instance's public IP, when it has one."
  value       = try(google_compute_instance.this.network_interface[0].access_config[0].nat_ip, null)
}
