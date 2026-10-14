# VirtualBox has no login: the provider runs VBoxManage on the machine
# that runs OpenTofu. The panel's backend has no VirtualBox, so a plan
# here fails: copy this code to a machine that has VirtualBox and run
# tofu there, or use the ESXi, vCenter or Proxmox template instead.
provider "virtualbox" {}
