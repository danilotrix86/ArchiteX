// Phase 9 / v1.4 isolated fixture for storage_account_public.
//
// Adds a brand-new public storage account on top of the base private
// one. The rule scans AddedNodes only (mirroring s3_bucket_public_exposure),
// so it fires on the new account only. Expected reasons:
//
//   storage_account_public       4.0
//   first_time_resource_type     (only when a baseline is supplied)
//   new_data_resource is NOT triggered because storage maps to "storage",
//   not "data".
//
// The existing private account stays untouched so we exercise "rule is
// silent on private storage" alongside "rule fires on public storage".

resource "azurerm_storage_account" "data" {
  name                     = "azurestoragebase01"
  resource_group_name      = "azure-storage-rg"
  location                 = "westeurope"
  account_tier             = "Standard"
  account_replication_type = "LRS"

  public_network_access_enabled   = false
  allow_nested_items_to_be_public = false
}

// New public storage account -> storage_account_public (4.0).
resource "azurerm_storage_account" "public_data" {
  name                     = "azurestoragepublic01"
  resource_group_name      = "azure-storage-rg"
  location                 = "westeurope"
  account_tier             = "Standard"
  account_replication_type = "LRS"

  public_network_access_enabled   = true
  allow_nested_items_to_be_public = false
}
