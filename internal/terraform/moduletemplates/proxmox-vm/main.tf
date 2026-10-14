# One VM per entry of var.vms (see vms.auto.tfvars), each cloned by the
# reusable module in modules/proxmox-vm.
module "vm" {
  source   = "./modules/proxmox-vm"
  for_each = var.vms

  name            = each.key
  node_name       = each.value.node_name
  template_vm_id  = each.value.template_vm_id
  vm_id           = each.value.vm_id
  datastore_id    = each.value.datastore_id
  cores           = each.value.cores
  memory_mb       = each.value.memory_mb
  disk_size       = each.value.disk_size
  bridge          = each.value.bridge
  vlan_id         = each.value.vlan_id
  ipv4_address    = each.value.ipv4_address
  ipv4_gateway    = each.value.ipv4_gateway
  username        = each.value.username
  ssh_public_keys = each.value.ssh_public_keys
  tags            = each.value.tags
}
