# One instance (and firewall rule) per entry of var.vms (see
# vms.auto.tfvars), each from the reusable module in modules/gcp-vm.
module "vm" {
  source   = "./modules/gcp-vm"
  for_each = var.vms

  name           = each.key
  zone           = each.value.zone
  machine_type   = each.value.machine_type
  image          = each.value.image
  disk_size_gb   = each.value.disk_size_gb
  network        = each.value.network
  subnetwork     = each.value.subnetwork
  public_ip      = each.value.public_ip
  ingress        = each.value.ingress
  ssh_user       = each.value.ssh_user
  ssh_public_key = each.value.ssh_public_key
  startup_script = each.value.startup_script
  labels         = each.value.labels
}
