data "vsphere_datacenter" "this" {
  name = var.datacenter
}

data "vsphere_compute_cluster" "this" {
  name          = var.cluster
  datacenter_id = data.vsphere_datacenter.this.id
}

data "vsphere_datastore" "this" {
  name          = var.datastore
  datacenter_id = data.vsphere_datacenter.this.id
}

data "vsphere_network" "this" {
  name          = var.network
  datacenter_id = data.vsphere_datacenter.this.id
}

data "vsphere_virtual_machine" "template" {
  name          = var.template
  datacenter_id = data.vsphere_datacenter.this.id
}

resource "vsphere_virtual_machine" "this" {
  name             = var.name
  folder           = var.folder != "" ? var.folder : null
  resource_pool_id = data.vsphere_compute_cluster.this.resource_pool_id
  datastore_id     = data.vsphere_datastore.this.id
  annotation       = var.annotation

  num_cpus  = var.num_cpus
  memory    = var.memory
  guest_id  = data.vsphere_virtual_machine.template.guest_id
  firmware  = data.vsphere_virtual_machine.template.firmware
  scsi_type = data.vsphere_virtual_machine.template.scsi_type

  network_interface {
    network_id   = data.vsphere_network.this.id
    adapter_type = data.vsphere_virtual_machine.template.network_interface_types[0]
  }

  disk {
    label            = "disk0"
    size             = max(var.disk_size, data.vsphere_virtual_machine.template.disks[0].size)
    thin_provisioned = data.vsphere_virtual_machine.template.disks[0].thin_provisioned
    eagerly_scrub    = data.vsphere_virtual_machine.template.disks[0].eagerly_scrub
  }

  clone {
    template_uuid = data.vsphere_virtual_machine.template.id
    linked_clone  = var.linked_clone

    customize {
      linux_options {
        host_name = var.name
        domain    = var.domain
      }

      network_interface {
        ipv4_address = var.ipv4_address != "" ? var.ipv4_address : null
        ipv4_netmask = var.ipv4_address != "" ? var.ipv4_netmask : null
      }

      ipv4_gateway    = var.ipv4_gateway != "" ? var.ipv4_gateway : null
      dns_server_list = var.dns_servers
    }
  }
}
