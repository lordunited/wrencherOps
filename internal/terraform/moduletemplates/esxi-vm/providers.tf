# The ESXi host is reached over SSH (enable it in the host's settings) and
# its HTTPS API. Set the values on the Variables tab.
provider "esxi" {
  esxi_hostname = var.esxi_hostname
  esxi_hostport = tostring(var.esxi_ssh_port)
  esxi_hostssl  = tostring(var.esxi_https_port)
  esxi_username = var.esxi_username
  esxi_password = var.esxi_password
}
