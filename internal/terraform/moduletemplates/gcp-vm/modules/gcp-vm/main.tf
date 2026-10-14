locals {
  metadata = merge(
    var.ssh_public_key != "" ? { ssh-keys = "${var.ssh_user}:${var.ssh_public_key}" } : {},
    var.startup_script != "" ? { startup-script = var.startup_script } : {},
  )
}

resource "google_compute_instance" "this" {
  name         = var.name
  zone         = var.zone
  machine_type = var.machine_type
  tags         = [var.name]
  labels       = merge(var.labels, { managed-by = "opentofu" })
  metadata     = local.metadata

  boot_disk {
    initialize_params {
      image = var.image
      size  = var.disk_size_gb
      type  = "pd-balanced"
    }
  }

  network_interface {
    network    = var.network
    subnetwork = var.subnetwork != "" ? var.subnetwork : null

    dynamic "access_config" {
      for_each = var.public_ip ? [1] : []
      content {}
    }
  }

  shielded_instance_config {
    enable_secure_boot          = true
    enable_vtpm                 = true
    enable_integrity_monitoring = true
  }

  lifecycle {
    # A newer image in the family must not replace a running instance.
    ignore_changes = [boot_disk[0].initialize_params[0].image]
  }
}

resource "google_compute_firewall" "ingress" {
  count       = length(var.ingress) > 0 ? 1 : 0
  name        = "${var.name}-ingress"
  network     = var.network
  target_tags = [var.name]

  allow {
    protocol = "tcp"
    ports    = distinct([for r in var.ingress : tostring(r.port)])
  }

  source_ranges = distinct([for r in var.ingress : r.source])
}
