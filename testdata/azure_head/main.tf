// Phase 9 / v1.4 head state (Azure tranche-0). Expected risk score
// (deterministic, asserted by tests):
//
//   nsg_allow_all_ingress    3.5  (the * inbound NSG rule was ADDED)
//   new_entry_point          3.0  (the new public LB was ADDED)
//   mssql_database_public    3.5  (new public MSSQL server was ADDED)
//
//   raw sum = 10.0 -> HIGH
//
// public_exposure_introduced does NOT fire here: the LB / public IP are
// freshly ADDED (their public:true is a creation property, not a change
// from false to true). new_entry_point alone catches the addition.
// new_data_resource fires once per added "data" node; the new MSSQL
// server is "data" (per AbstractionMap), so we expect that too:
//
//   new_data_resource         2.5  (azurerm_mssql_server.public_app)
//
// Total raw = 12.5 -> capped at 10.0 -> HIGH.
//
// The first_time_* baseline rules add nothing here because no baseline
// is provided in the regression test (consistent with the v1.1 / v1.2
// regression fixtures).

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

// --- Newly introduced resources ---

// Opens the subnet at the network layer -> nsg_allow_all_ingress (3.5).
resource "azurerm_network_security_rule" "open_world" {
  name                        = "allow-world-inbound"
  resource_group_name         = "azure-base-rg"
  network_security_group_name = azurerm_network_security_group.main.name
  priority                    = 100
  direction                   = "Inbound"
  access                      = "Allow"
  protocol                    = "Tcp"
  source_port_range           = "*"
  destination_port_range      = "443"
  source_address_prefix       = "*"
  destination_address_prefix  = "*"
}

// Public IP attached to the LB -> public:true on both nodes.
resource "azurerm_public_ip" "lb" {
  name                = "azure-head-pip"
  resource_group_name = "azure-base-rg"
  location            = "westeurope"
  allocation_method   = "Static"
  sku                 = "Standard"
}

// New entry point -> new_entry_point (3.0) + public_exposure_introduced
// (4.0) since LB carries public:true via deriveAttributes.
resource "azurerm_lb" "front" {
  name                = "azure-head-lb"
  resource_group_name = "azure-base-rg"
  location            = "westeurope"
  sku                 = "Standard"

  frontend_ip_configuration {
    name                 = "public-frontend"
    public_ip_address_id = azurerm_public_ip.lb.id
  }
}

// New publicly-reachable MSSQL server -> mssql_database_public (3.5)
// + new_data_resource (2.5). The base server stays private; the rule
// fires on this newly-added one only.
resource "azurerm_mssql_server" "public_app" {
  name                          = "azure-head-public-mssql"
  resource_group_name           = "azure-base-rg"
  location                      = "westeurope"
  version                       = "12.0"
  administrator_login           = "sqladmin"
  administrator_login_password  = "Replaced-In-Real-Use"
  public_network_access_enabled = true
}

// Database on the new public server. Adds an extra new_data_resource
// finding (2.5) which is capped by the global 10.0 score ceiling.
resource "azurerm_mssql_database" "app" {
  name      = "azure-head-db"
  server_id = azurerm_mssql_server.public_app.id
}
