# One VM per entry of var.vms (see vms.auto.tfvars), each from the
# reusable module in modules/esxi-vm.
module "vm" {
  source   = "./modules/esxi-vm"
  for_each = var.vms

  name           = each.key
  disk_store     = each.value.disk_store
  clone_from_vm  = each.value.clone_from_vm
  guestos        = each.value.guestos
  numvcpus       = each.value.numvcpus
  memsize        = each.value.memsize
  boot_disk_size = each.value.boot_disk_size
  boot_disk_type = each.value.boot_disk_type
  networks       = each.value.networks
  power          = each.value.power
  notes          = each.value.notes
}
