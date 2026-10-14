output "id" {
  description = "The container's ID."
  value       = docker_container.this.id
}

output "image_id" {
  description = "The ID of the image the container runs."
  value       = docker_image.this.image_id
}
