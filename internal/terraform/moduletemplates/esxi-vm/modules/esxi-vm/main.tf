resource "esxi_guest" "this" {
  guest_name     = var.name
  disk_store     = var.disk_store
  clone_from_vm  = var.clone_from_vm != "" ? var.clone_from_vm : null
  guestos        = var.clone_from_vm == "" ? var.guestos : null
  numvcpus       = tostring(var.numvcpus)
  memsize        = tostring(var.memsize)
  boot_disk_size = tostring(var.boot_disk_size)
  boot_disk_type = var.boot_disk_type
  power          = var.power
  notes          = var.notes

  # Waits for VMware Tools to report an IP; an empty VM has none, so
  # it gives up after this many seconds and leaves ip_address empty.
  guest_startup_timeout = 120

  dynamic "network_interfaces" {
    for_each = var.networks
    content {
      virtual_network = network_interfaces.value
    }
  }
}
