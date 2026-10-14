terraform {
  required_version = ">= 1.6"

  required_providers {
    gitlab = {
      source  = "gitlabhq/gitlab"
      version = "~> 19.4"
    }
  }
}
