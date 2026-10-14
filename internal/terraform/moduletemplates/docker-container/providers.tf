# The Docker host. From the panel, use the host's TLS port: docker_host =
# "tcp://host:2376" with the client certificate on the Variables tab (see
# Docker's "Protect the Docker daemon socket"). ssh://user@host also works
# where OpenTofu runs with an SSH key; the panel has none.
provider "docker" {
  host          = var.docker_host
  ca_material   = var.docker_ca_cert != "" ? var.docker_ca_cert : null
  cert_material = var.docker_client_cert != "" ? var.docker_client_cert : null
  key_material  = var.docker_client_private_key != "" ? var.docker_client_private_key : null
}
