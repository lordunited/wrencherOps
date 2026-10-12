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
// whole flow; the blank one is the usual layout to fill in.
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
}
