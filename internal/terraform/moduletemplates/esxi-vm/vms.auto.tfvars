# The VMs this workspace manages: rename, add or remove entries, then plan.
# Only disk_store is required; see variables.tf for every option.
vms = {
  "web-01" = {
    disk_store    = "datastore1"
    clone_from_vm = "ubuntu-template"
    numvcpus      = 2
    memsize       = 4096
  }
}
