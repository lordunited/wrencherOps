resource "azurerm_resource_group" "this" {
  name     = var.resource_group_name
  location = var.location
  tags     = var.tags
}

resource "azurerm_virtual_network" "this" {
  name                = "${var.resource_group_name}-vnet"
  resource_group_name = azurerm_resource_group.this.name
  location            = azurerm_resource_group.this.location
  address_space       = [var.address_space]
  tags                = var.tags
}

resource "azurerm_subnet" "vms" {
  name                 = "vms"
  resource_group_name  = azurerm_resource_group.this.name
  virtual_network_name = azurerm_virtual_network.this.name
  address_prefixes     = [var.subnet_prefix]
}

# One VM per entry of var.vms (see vms.auto.tfvars), each from the
# reusable module in modules/azure-vm.
module "vm" {
  source   = "./modules/azure-vm"
  for_each = var.vms

  name                = each.key
  resource_group_name = azurerm_resource_group.this.name
  location            = azurerm_resource_group.this.location
  subnet_id           = azurerm_subnet.vms.id
  size                = each.value.size
  admin_username      = each.value.admin_username
  ssh_public_key      = each.value.ssh_public_key
  public_ip           = each.value.public_ip
  os_disk_gb          = each.value.os_disk_gb
  ingress             = each.value.ingress
  custom_data         = each.value.custom_data
  tags                = var.tags
}
