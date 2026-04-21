// Phase 9 / v1.4 example. The base is a closed Azure footprint: a
// virtual network, a private subnet, an NSG that does not yet open
// anything to the world, and an MSSQL server with public network
// access disabled.
//
// The matching head fixture introduces the canonical "public LB +
// world-inbound NSG rule" anti-pattern -- the Azure equivalent of
// example 01 (public ALB).

resource "azurerm_virtual_network" "main" {
  name                = "ex07-vnet"
  resource_group_name = "ex07-rg"
  location            = "westeurope"
  address_space       = ["10.20.0.0/16"]
}

resource "azurerm_subnet" "main" {
  name                 = "ex07-subnet"
  resource_group_name  = "ex07-rg"
  virtual_network_name = azurerm_virtual_network.main.name
  address_prefixes     = ["10.20.1.0/24"]
}

resource "azurerm_network_security_group" "main" {
  name                = "ex07-nsg"
  resource_group_name = "ex07-rg"
  location            = "westeurope"
}

resource "azurerm_mssql_server" "main" {
  name                          = "ex07-mssql"
  resource_group_name           = "ex07-rg"
  location                      = "westeurope"
  version                       = "12.0"
  administrator_login           = "sqladmin"
  administrator_login_password  = "Replaced-In-Real-Use"
  public_network_access_enabled = false
}
