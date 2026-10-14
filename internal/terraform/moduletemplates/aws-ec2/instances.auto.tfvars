# The instances this workspace launches: rename, add or remove entries,
# then plan. Without ami_id it boots the latest Ubuntu 24.04; without
# subnet_id it uses the default VPC.
instances = {
  "web-01" = {
    instance_type = "t3.small"
    public_ip     = true
    ingress = [
      { port = 443, cidr = "0.0.0.0/0", description = "HTTPS" },
      { port = 22, cidr = "203.0.113.0/24", description = "SSH from the office" },
    ]
  }
}
