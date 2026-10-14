// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package terraform

// Template is a starter workspace.
type Template struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Files       []File `json:"files"`
}

// Templates. The example uses providers that need no account (random,
// local, null), so it can be planned and applied right away to see the
// whole flow; the blank one is the usual layout to fill in. The providers
// one pins versions found on the OpenTofu registry on 2026-10-12.
var Templates = []Template{
	{
		Key: "blank", Name: "Blank workspace", Description: "The usual layout: versions, variables, main and outputs.",
		Files: []File{
			{Path: "versions.tf", Content: `terraform {
  required_version = ">= 1.6"

  required_providers {
    # e.g. aws = { source = "hashicorp/aws", version = "~> 6.0" }
  }
}
`},
			{Path: "variables.tf", Content: "# Input variables. Set their values on the Variables tab.\n"},
			{Path: "main.tf", Content: "# Resources.\n"},
			{Path: "outputs.tf", Content: "# Outputs, shown after an apply.\n"},
		},
	},
	{
		Key: "example", Name: "Try it: a no-account example", Description: "A random name and a null resource — no cloud account needed. Plan and apply it to see the whole flow, then destroy it.",
		Files: []File{
			{Path: "versions.tf", Content: `terraform {
  required_version = ">= 1.6"

  required_providers {
    random = { source = "hashicorp/random", version = "~> 3.6" }
    null   = { source = "hashicorp/null", version = "~> 3.2" }
  }
}
`},
			{Path: "variables.tf", Content: `variable "environment" {
  description = "Where this runs, e.g. staging or production."
  type        = string
  default     = "staging"
}
`},
			{Path: "main.tf", Content: `resource "random_pet" "name" {
  prefix = var.environment
}

resource "null_resource" "announce" {
  triggers = {
    name = random_pet.name.id
  }
}
`},
			{Path: "outputs.tf", Content: `output "name" {
  description = "The random name this workspace made."
  value       = random_pet.name.id
}
`},
		},
	},
	{
		Key: "providers", Name: "Popular providers and modules", Description: "ESXi, vCenter, VirtualBox, Proxmox, libvirt, Vault, AWS, Azure, Google, Kubernetes and more, pinned — plus a catalog of popular modules. Delete the providers you don't use: each one is downloaded on every init.",
		Files: []File{
			{Path: "versions.tf", Content: `terraform {
  required_version = ">= 1.6"

  required_providers {
    # --- Virtualization ---
    esxi = {
      source  = "josenk/esxi" # standalone ESXi host, no vCenter needed
      version = "~> 1.10"
    }
    vsphere = {
      source  = "hashicorp/vsphere" # vCenter / vSphere
      version = "~> 2.17"
    }
    virtualbox = {
      source  = "terra-farm/virtualbox" # runs VBoxManage where OpenTofu runs, so not inside the panel
      version = "0.2.2-alpha.1"         # only pre-releases exist, and a pre-release must be pinned exactly
    }
    proxmox = {
      source  = "bpg/proxmox" # Proxmox VE
      version = "~> 0.116.0"
    }
    libvirt = {
      source  = "dmacvicar/libvirt" # KVM / QEMU
      version = "~> 0.9.9"
    }

    # --- Secrets ---
    vault = {
      source  = "hashicorp/vault" # HashiCorp Vault
      version = "~> 5.12"
    }

    # --- Public cloud ---
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.68"
    }
    azurerm = {
      source  = "hashicorp/azurerm"
      version = "~> 5.9"
    }
    google = {
      source  = "hashicorp/google"
      version = "~> 8.6"
    }
    cloudflare = {
      source  = "cloudflare/cloudflare"
      version = "~> 5.27"
    }

    # --- Containers and Kubernetes ---
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = "~> 3.3"
    }
    helm = {
      source  = "hashicorp/helm"
      version = "~> 3.3"
    }
    docker = {
      source  = "kreuzwerker/docker"
      version = "~> 4.6"
    }

    # --- DevOps tools ---
    gitlab = {
      source  = "gitlabhq/gitlab"
      version = "~> 19.4"
    }

    # --- Utilities (no account needed) ---
    random = {
      source  = "hashicorp/random"
      version = "~> 3.9"
    }
    tls = {
      source  = "hashicorp/tls"
      version = "~> 4.4"
    }
    null = {
      source  = "hashicorp/null"
      version = "~> 3.3"
    }
    local = {
      source  = "hashicorp/local"
      version = "~> 2.9"
    }
  }
}
`},
			{Path: "providers.tf", Content: `# How each provider logs in. Uncomment the ones you keep.
#
# vCenter and Vault read their login from environment variables: add them
# on the Variables tab as environment variables, the secret ones sensitive.
#
# provider "vsphere" {} # VSPHERE_SERVER, VSPHERE_USER, VSPHERE_PASSWORD, VSPHERE_ALLOW_UNVERIFIED_SSL
# provider "vault" {}   # VAULT_ADDR, VAULT_TOKEN
#
# ESXi needs SSH enabled on the host, and Terraform variables (declare
# esxi_hostname and esxi_password in variables.tf, the password sensitive):
#
# provider "esxi" {
#   esxi_hostname = var.esxi_hostname
#   esxi_hostport = "22"
#   esxi_hostssl  = "443"
#   esxi_username = "root"
#   esxi_password = var.esxi_password
# }
`},
			{Path: "modules.tf", Content: `# Popular registry modules, pinned. Uncomment one, give it its inputs,
# and press Format.
#
# module "vpc" {
#   source  = "terraform-aws-modules/vpc/aws"
#   version = "~> 6.7"
# }
# module "eks" {
#   source  = "terraform-aws-modules/eks/aws"
#   version = "~> 21.29"
# }
# module "ec2_instance" {
#   source  = "terraform-aws-modules/ec2-instance/aws"
#   version = "~> 6.4"
# }
# module "s3_bucket" {
#   source  = "terraform-aws-modules/s3-bucket/aws"
#   version = "~> 5.17"
# }
# module "rds" {
#   source  = "terraform-aws-modules/rds/aws"
#   version = "~> 7.2"
# }
# module "security_group" {
#   source  = "terraform-aws-modules/security-group/aws"
#   version = "~> 6.0"
# }
# module "iam" {
#   source  = "terraform-aws-modules/iam/aws"
#   version = "~> 6.8"
# }
# module "aks" {
#   source  = "Azure/aks/azurerm"
#   version = "~> 11.7"
# }
# module "vnet" {
#   source  = "Azure/avm-res-network-virtualnetwork/azurerm"
#   version = "~> 0.22"
# }
# module "gcp_network" {
#   source  = "terraform-google-modules/network/google"
#   version = "~> 18.3"
# }
# module "gke" {
#   source  = "terraform-google-modules/kubernetes-engine/google"
#   version = "~> 45.0"
# }
# module "vsphere_vm" {
#   source  = "Terraform-VMWare-Modules/vm/vsphere"
#   version = "~> 3.8"
# }
# module "label" {
#   source  = "cloudposse/label/null"
#   version = "~> 0.25"
# }
`},
			{Path: "variables.tf", Content: "# Input variables. Set their values on the Variables tab.\n"},
			{Path: "main.tf", Content: "# Resources.\n"},
		},
	},
}
