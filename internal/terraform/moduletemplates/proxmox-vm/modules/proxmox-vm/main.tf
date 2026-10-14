resource "proxmox_virtual_environment_vm" "this" {
  name            = var.name
  node_name       = var.node_name
  vm_id           = var.vm_id
  tags            = var.tags
  description     = "Managed by OpenTofu"
  started         = true
  on_boot         = true
  stop_on_destroy = true

  clone {
    vm_id = var.template_vm_id
    full  = true
  }

  # Reports the VM's IPs back; the template needs qemu-guest-agent.
  agent {
    enabled = true
  }

  cpu {
    cores = var.cores
    type  = "host"
  }

  memory {
    dedicated = var.memory_mb
  }

  disk {
    datastore_id = var.datastore_id
    interface    = "scsi0"
    size         = var.disk_size
    discard      = "on"
  }

  network_device {
    bridge  = var.bridge
    model   = "virtio"
    vlan_id = var.vlan_id
  }

  initialization {
    datastore_id = var.datastore_id

    ip_config {
      ipv4 {
        address = var.ipv4_address
        gateway = var.ipv4_address != "dhcp" && var.ipv4_gateway != "" ? var.ipv4_gateway : null
      }
    }

    user_account {
      username = var.username
      keys     = var.ssh_public_keys
    }
  }
}
