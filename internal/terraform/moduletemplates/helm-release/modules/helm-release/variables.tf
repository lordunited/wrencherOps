variable "name" {
  type        = string
  description = "The release's name."
}

variable "namespace" {
  type        = string
  description = "The namespace the release is installed in."
}

variable "create_namespace" {
  type        = bool
  description = "Create the namespace if it doesn't exist."
  default     = true
}

variable "repository" {
  type        = string
  description = "The chart repository's URL, e.g. https://charts.bitnami.com/bitnami, or oci://registry/path."
}

variable "chart" {
  type        = string
  description = "The chart's name in the repository, e.g. redis."
}

variable "chart_version" {
  type        = string
  description = "The chart version, pinned, e.g. 20.1.3."

  validation {
    condition     = var.chart_version != "" && var.chart_version != "latest"
    error_message = "Pin chart_version, so every apply installs the same chart."
  }
}

variable "values_yaml" {
  type        = string
  description = "The release's values, as a YAML document. Empty for the chart's defaults."
  default     = ""
}

variable "secret_values" {
  type        = map(string)
  description = "Secret values, by their dotted path in the chart, e.g. { \"auth.password\" = \"...\" }."
  sensitive   = true
  default     = {}
}

variable "timeout" {
  type        = number
  description = "How long to wait for the release's resources to be ready, in seconds."
  default     = 600
}
