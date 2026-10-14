# One VM per entry of var.vms (see vms.auto.tfvars), each from the
# reusable module in modules/virtualbox-vm.
module "vm" {
  source   = "./modules/virtualbox-vm"
  for_each = var.vms

  name           = each.key
  image          = each.value.image
  cpus           = each.value.cpus
  memory_mb      = each.value.memory_mb
  network_type   = each.value.network_type
  host_interface = each.value.host_interface
  user_data      = each.value.user_data
}
