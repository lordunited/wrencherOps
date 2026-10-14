# The instances this workspace manages: rename, add or remove entries,
# then plan. Names must be lowercase letters, digits and dashes.
vms = {
  "web-01" = {
    zone         = "europe-west3-a"
    machine_type = "e2-small"
    public_ip    = true
    ingress = [
      { port = 443, source = "0.0.0.0/0" },
    ]
  }
}
