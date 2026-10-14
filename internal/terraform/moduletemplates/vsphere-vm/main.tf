# One VM per entry of var.vms (see vms.auto.tfvars), each cloned by the
# reusable module in modules/vsphere-vm.
module "vm" {
  source   = "./modules/vsphere-vm"
  for_each = var.vms

  name         = each.key
  datacenter   = each.value.datacenter
  cluster      = each.value.cluster
  datastore    = each.value.datastore
  network      = each.value.network
  template     = each.value.template
  folder       = each.value.folder
  num_cpus     = each.value.num_cpus
  memory       = each.value.memory
  disk_size    = each.value.disk_size
  linked_clone = each.value.linked_clone
  domain       = each.value.domain
  ipv4_address = each.value.ipv4_address
  ipv4_netmask = each.value.ipv4_netmask
  ipv4_gateway = each.value.ipv4_gateway
  dns_servers  = each.value.dns_servers
  annotation   = each.value.annotation
}
