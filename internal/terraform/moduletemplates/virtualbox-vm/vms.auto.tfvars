# The VMs this workspace manages: rename, add or remove entries, then plan.
# image is a Vagrant .box; see variables.tf for every option.
vms = {
  "dev-01" = {
    image     = "https://app.vagrantup.com/ubuntu/boxes/jammy64/versions/20241002.0.0/providers/virtualbox.box"
    cpus      = 2
    memory_mb = 2048
  }
}
