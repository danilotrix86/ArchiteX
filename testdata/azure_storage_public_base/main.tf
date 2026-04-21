// Phase 9 / v1.4 isolated fixture. Establishes a private storage
// account so the head fixture can flip exactly one flag and isolate
// the storage_account_public rule.

resource "azurerm_storage_account" "data" {
  name                     = "azurestoragebase01"
  resource_group_name      = "azure-storage-rg"
  location                 = "westeurope"
  account_tier             = "Standard"
  account_replication_type = "LRS"

  public_network_access_enabled   = false
  allow_nested_items_to_be_public = false
}
