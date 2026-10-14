terraform {
  required_version = ">= 1.6"

  required_providers {
    vsphere = {
      source  = "hashicorp/vsphere"
      version = "~> 2.17"
    }
  }
}
