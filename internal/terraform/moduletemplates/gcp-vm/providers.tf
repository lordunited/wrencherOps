# A service account key comes from an environment variable: on the
# Variables tab add GOOGLE_CREDENTIALS as an environment variable, marked
# sensitive, holding the key file's JSON (a service account with the
# Compute Admin role).
provider "google" {
  project = var.gcp_project
  region  = var.gcp_region
}
