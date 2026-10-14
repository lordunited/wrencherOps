# A service principal's login comes from environment variables: on the
# Variables tab add ARM_TENANT_ID, ARM_CLIENT_ID and ARM_CLIENT_SECRET as
# environment variables (the secret one sensitive). Create one with
# az ad sp create-for-rbac --role Contributor --scopes /subscriptions/<id>
provider "azurerm" {
  features {}
  subscription_id = var.azure_subscription_id
}
