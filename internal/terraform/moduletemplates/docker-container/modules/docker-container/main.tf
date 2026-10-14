resource "docker_image" "this" {
  name         = var.image
  keep_locally = true
}

resource "docker_container" "this" {
  name     = var.name
  image    = docker_image.this.image_id
  restart  = var.restart
  env      = [for k, v in var.env : "${k}=${v}"]
  command  = length(var.command) > 0 ? var.command : null
  memory   = var.memory_mb > 0 ? var.memory_mb : null
  cpus     = var.cpus != "" ? var.cpus : null
  must_run = true

  dynamic "ports" {
    for_each = var.ports
    content {
      internal = ports.value.internal
      external = ports.value.external
      protocol = ports.value.protocol
      ip       = ports.value.ip
    }
  }

  dynamic "volumes" {
    for_each = var.volumes
    content {
      container_path = volumes.value.container_path
      host_path      = volumes.value.host_path
      volume_name    = volumes.value.volume_name
      read_only      = volumes.value.read_only
    }
  }

  dynamic "healthcheck" {
    for_each = length(var.healthcheck) > 0 ? [var.healthcheck] : []
    content {
      test     = healthcheck.value
      interval = "30s"
      timeout  = "5s"
      retries  = 3
    }
  }

  labels {
    label = "managed-by"
    value = "opentofu"
  }

  dynamic "labels" {
    for_each = var.labels
    content {
      label = labels.key
      value = labels.value
    }
  }
}
