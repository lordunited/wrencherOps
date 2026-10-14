# The VMs this workspace manages: rename, add or remove entries, then plan.
# See variables.tf for every option; leave ipv4_address out for DHCP.
vms = {
  "app-01" = {
    datacenter   = "DC1"
    cluster      = "Cluster1"
    datastore    = "datastore1"
    network      = "VM Network"
    template     = "ubuntu-24.04-template"
    num_cpus     = 2
    memory       = 4096
    domain       = "corp.local"
    ipv4_address = "10.0.10.21"
    ipv4_netmask = 24
    ipv4_gateway = "10.0.10.1"
    dns_servers  = ["10.0.10.2"]
  }
}
