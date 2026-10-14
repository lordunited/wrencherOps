# The VMs this workspace manages: rename, add or remove entries, then plan.
# See variables.tf for every option; ipv4_address = "dhcp" for DHCP.
vms = {
  "app-01" = {
    node_name       = "pve"
    template_vm_id  = 9000
    cores           = 2
    memory_mb       = 4096
    disk_size       = 32
    ipv4_address    = "10.0.20.31/24"
    ipv4_gateway    = "10.0.20.1"
    ssh_public_keys = ["ssh-ed25519 AAAA... you@laptop"]
  }
}
