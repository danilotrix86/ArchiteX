// Phase 9 / v1.4 example. A developer adds a public Azure load balancer
// AND opens the network-security group from anywhere on the internet.
// This is the Azure equivalent of example 01 (public ALB) and triggers
// the same exposure-class signals across both providers.

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

// NEW: opens the subnet at the network layer -> nsg_allow_all_ingress.
resource "azurerm_network_security_rule" "open_world" {
  name                        = "allow-world-https"
  resource_group_name         = "ex07-rg"
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

// NEW: routable address that backs the public LB -> public:true.
resource "azurerm_public_ip" "lb" {
  name                = "ex07-pip"
  resource_group_name = "ex07-rg"
  location            = "westeurope"
  allocation_method   = "Static"
  sku                 = "Standard"
}

// NEW: public load balancer -> new_entry_point + public_exposure_introduced.
resource "azurerm_lb" "front" {
  name                = "ex07-lb"
  resource_group_name = "ex07-rg"
  location            = "westeurope"
  sku                 = "Standard"

  frontend_ip_configuration {
    name                 = "public-frontend"
    public_ip_address_id = azurerm_public_ip.lb.id
  }
}
