terraform {
  required_version = ">= 1.6"

  required_providers {
    virtualbox = {
      source  = "terra-farm/virtualbox"
      version = "0.2.2-alpha.1" # only pre-releases exist, and a pre-release must be pinned exactly
    }
  }
}
