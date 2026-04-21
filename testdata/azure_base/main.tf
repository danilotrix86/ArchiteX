// Phase 9 / v1.4 baseline (Azure tranche-0). Establishes a minimal,
// safe Azure footprint that the matching head fixture builds on top of:
//
//   - a virtual network with one subnet
//   - a network-security group attached to that subnet
//   - an existing internal MSSQL server (public network access OFF)
//
// The head fixture introduces the unsafe variants for every Azure-side
// rule (nsg_allow_all_ingress, storage_account_public,
// mssql_database_public) PLUS a public LB so the cross-provider rules
// (new_entry_point, public_exposure_introduced) also fire. This mirrors
// the AWS top10_base / top10_head and tranche3_base / tranche3_head
// pairing approach.
//
// The resource group is referenced by name only (it is intentionally
// outside the supported registry -- it has no architectural-review
// value, and including it would clutter every Azure diagram with an
// inert root node).

resource "azurerm_virtual_network" "main" {
  name                = "azure-base-vnet"
  resource_group_name = "azure-base-rg"
  location            = "westeurope"
  address_space       = ["10.0.0.0/16"]
}

resource "azurerm_subnet" "main" {
  name                 = "azure-base-subnet"
  resource_group_name  = "azure-base-rg"
  virtual_network_name = azurerm_virtual_network.main.name
  address_prefixes     = ["10.0.1.0/24"]
}

resource "azurerm_network_security_group" "main" {
  name                = "azure-base-nsg"
  resource_group_name = "azure-base-rg"
  location            = "westeurope"
}

resource "azurerm_mssql_server" "main" {
  name                          = "azure-base-mssql"
  resource_group_name           = "azure-base-rg"
  location                      = "westeurope"
  version                       = "12.0"
  administrator_login           = "sqladmin"
  administrator_login_password  = "Replaced-In-Real-Use"
  public_network_access_enabled = false
}
