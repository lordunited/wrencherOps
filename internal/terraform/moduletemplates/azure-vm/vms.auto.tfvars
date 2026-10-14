# The VMs this workspace manages: rename, add or remove entries, then plan.
# Each boots Ubuntu 24.04 with SSH key login only.
vms = {
  "web-01" = {
    ssh_public_key = "ssh-ed25519 AAAA... you@laptop"
    size           = "Standard_B2s"
    ingress = [
      { port = 443, source = "Internet" },
      { port = 22, source = "203.0.113.0/24" },
    ]
  }
}
