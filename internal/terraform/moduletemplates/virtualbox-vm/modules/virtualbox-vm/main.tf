resource "virtualbox_vm" "this" {
  name      = var.name
  image     = var.image
  cpus      = var.cpus
  memory    = "${var.memory_mb} mib"
  user_data = var.user_data != "" ? var.user_data : null

  network_adapter {
    type           = var.network_type
    host_interface = var.network_type == "nat" ? null : var.host_interface
  }
}
