terraform {
  required_version = ">= 1.6"

  required_providers {
    esxi = {
      source  = "josenk/esxi"
      version = "~> 1.10"
    }
  }
}
