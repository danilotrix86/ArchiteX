package registry

import "strings"

// This file owns every azurerm_* resource registration: supported
// types + abstract role + edge labels + attribute promoter. Mirrors
// aws.go in shape; see that file for the design rationale.

func init() {
	registerAzureResources()
	registerAzureEdges()
}

func registerAzureResources() {
	// v1.4 -- Azure tranche-0 (Phase 9). First-class Azure (azurerm)
	// support. azurerm_resource_group is INTENTIONALLY excluded -- it
	// is purely organizational and would clutter every Azure diagram
	// with an inert root node. References to it from other resources
	// simply do not produce edges (warn-and-skip).
	Register(Resource{ProviderType: "azurerm_virtual_network", AbstractType: "network"})
	Register(Resource{ProviderType: "azurerm_subnet", AbstractType: "network"})
	Register(Resource{ProviderType: "azurerm_public_ip", AbstractType: "network", Promoter: promotePublicTrue})
	Register(Resource{ProviderType: "azurerm_network_security_group", AbstractType: "access_control"})
	Register(Resource{ProviderType: "azurerm_network_security_rule", AbstractType: "access_control", Promoter: promoteAzureNSGRule})
	Register(Resource{ProviderType: "azurerm_network_interface", AbstractType: "network"})
	Register(Resource{ProviderType: "azurerm_linux_virtual_machine", AbstractType: "compute"})
	Register(Resource{ProviderType: "azurerm_windows_virtual_machine", AbstractType: "compute"})
	Register(Resource{ProviderType: "azurerm_lb", AbstractType: "entry_point", Promoter: promotePublicTrue})
	Register(Resource{ProviderType: "azurerm_storage_account", AbstractType: "storage", Promoter: promoteAzureStorageAccount})
	Register(Resource{ProviderType: "azurerm_mssql_server", AbstractType: "data", Promoter: promoteAzureMSSQLServer})
	Register(Resource{ProviderType: "azurerm_mssql_database", AbstractType: "data"})
}

func registerAzureEdges() {
	// Network topology: subnets in vnet, NICs in subnets and bound to
	// NSGs / public IPs, LBs in subnets and bound to public IPs. NSG
	// rules attach to their parent NSG (mirrors
	// aws_security_group_rule -> aws_security_group). NSG itself
	// applies_to a subnet when associated.
	RegisterEdge("azurerm_subnet", "azurerm_virtual_network", "part_of")
	RegisterEdge("azurerm_network_interface", "azurerm_subnet", "deployed_in")
	RegisterEdge("azurerm_network_interface", "azurerm_network_security_group", "attached_to")
	RegisterEdge("azurerm_network_interface", "azurerm_public_ip", "attached_to")
	RegisterEdge("azurerm_lb", "azurerm_subnet", "deployed_in")
	RegisterEdge("azurerm_lb", "azurerm_public_ip", "attached_to")
	RegisterEdge("azurerm_network_security_group", "azurerm_subnet", "applies_to")
	RegisterEdge("azurerm_network_security_rule", "azurerm_network_security_group", "applies_to")
	// VM compute: VMs attach via NICs (Azure has no direct VM->subnet
	// reference; the NIC is the network anchor).
	RegisterEdge("azurerm_linux_virtual_machine", "azurerm_network_interface", "attached_to")
	RegisterEdge("azurerm_windows_virtual_machine", "azurerm_network_interface", "attached_to")
	// Data: an MSSQL database is part_of its parent server.
	RegisterEdge("azurerm_mssql_database", "azurerm_mssql_server", "part_of")
}

// promoteAzureNSGRule promotes the literal trio
// (source_address_prefix, access, direction) so nsg_allow_all_ingress
// can inspect them without re-parsing. `public` becomes true when the
// rule itself opens the world inbound (Allow + inbound + "*" or
// "0.0.0.0/0" prefix). Variable-driven values land as nil; the rule
// stays silent.
func promoteAzureNSGRule(attrs map[string]any) map[string]any {
	out := map[string]any{}
	for _, k := range []string{"source_address_prefix", "access", "direction"} {
		if v, ok := attrs[k]; ok {
			if s, ok := v.(string); ok && s != "" {
				out[k] = s
			}
		}
	}
	pub := false
	if src, ok := out["source_address_prefix"].(string); ok {
		if (src == "*" || src == "0.0.0.0/0") &&
			strings.EqualFold(asString(out["access"]), "allow") &&
			strings.EqualFold(asString(out["direction"]), "inbound") {
			pub = true
		}
	}
	out["public"] = pub
	return out
}

// promoteAzureStorageAccount passes the two flags
// storage_account_public reads through, and sets `public` to true if
// either is literal-true. Default-false posture: a missing attribute
// does NOT imply public access (azurerm defaults vary by version,
// and we never guess).
func promoteAzureStorageAccount(attrs map[string]any) map[string]any {
	out := map[string]any{}
	pub := false
	for _, k := range []string{"public_network_access_enabled", "allow_nested_items_to_be_public"} {
		if v, ok := attrs[k]; ok {
			if b, ok := v.(bool); ok {
				out[k] = b
				if b {
					pub = true
				}
			}
		}
	}
	out["public"] = pub
	return out
}

// promoteAzureMSSQLServer passes public_network_access_enabled
// through and uses it as the `public` value. Variable-driven values
// land as nil and the rule stays silent.
func promoteAzureMSSQLServer(attrs map[string]any) map[string]any {
	out := map[string]any{}
	pub := false
	if v, ok := attrs["public_network_access_enabled"]; ok {
		if b, ok := v.(bool); ok {
			out["public_network_access_enabled"] = b
			pub = b
		}
	}
	out["public"] = pub
	return out
}

// asString safely extracts a string from an attribute value. Used
// only by promoteAzureNSGRule to compare promoted-but-typed-loosely
// literals. Returns "" if the value is nil or not a string.
func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
