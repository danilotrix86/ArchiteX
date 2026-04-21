package risk

import (
	"testing"

	"architex/delta"
	"architex/models"
)

// ---------------------------------------------------------------------------
// nsg_allow_all_ingress
// ---------------------------------------------------------------------------

func TestEvaluate_NSG_AllowAllIngress_Wildcard_Fires(t *testing.T) {
	d := delta.Delta{
		AddedNodes: []models.Node{
			{
				ID:           "azurerm_network_security_rule.open",
				Type:         "access_control",
				ProviderType: "azurerm_network_security_rule",
				Attributes: map[string]any{
					"public":                true,
					"source_address_prefix": "*",
					"access":                "Allow",
					"direction":             "Inbound",
				},
			},
		},
		Summary: delta.DeltaSummary{AddedNodes: 1},
	}
	r := Evaluate(d)
	if !hasReason(r.Reasons, "nsg_allow_all_ingress") {
		t.Fatalf("expected nsg_allow_all_ingress, got %+v", r.Reasons)
	}
}

func TestEvaluate_NSG_AllowAllIngress_AnyCIDR_Fires(t *testing.T) {
	// "0.0.0.0/0" is the IPv4-equivalent expression of "*" and the
	// Azure docs treat them as synonymous for the source_address_prefix
	// field. Both must fire the rule.
	d := delta.Delta{
		AddedNodes: []models.Node{
			{
				ID:           "azurerm_network_security_rule.any_v4",
				Type:         "access_control",
				ProviderType: "azurerm_network_security_rule",
				Attributes: map[string]any{
					"public":                true,
					"source_address_prefix": "0.0.0.0/0",
					"access":                "Allow",
					"direction":             "Inbound",
				},
			},
		},
		Summary: delta.DeltaSummary{AddedNodes: 1},
	}
	r := Evaluate(d)
	if !hasReason(r.Reasons, "nsg_allow_all_ingress") {
		t.Fatalf("expected nsg_allow_all_ingress, got %+v", r.Reasons)
	}
}

func TestEvaluate_NSG_ScopedIngress_DoesNotFire(t *testing.T) {
	d := delta.Delta{
		AddedNodes: []models.Node{
			{
				ID:           "azurerm_network_security_rule.scoped",
				Type:         "access_control",
				ProviderType: "azurerm_network_security_rule",
				Attributes: map[string]any{
					"public":                false,
					"source_address_prefix": "10.0.0.0/8",
					"access":                "Allow",
					"direction":             "Inbound",
				},
			},
		},
		Summary: delta.DeltaSummary{AddedNodes: 1},
	}
	r := Evaluate(d)
	if hasReason(r.Reasons, "nsg_allow_all_ingress") {
		t.Fatalf("rule must NOT fire when source_address_prefix is scoped")
	}
}

func TestEvaluate_NSG_DenyAllIngress_DoesNotFire(t *testing.T) {
	d := delta.Delta{
		AddedNodes: []models.Node{
			{
				ID:           "azurerm_network_security_rule.deny",
				Type:         "access_control",
				ProviderType: "azurerm_network_security_rule",
				Attributes: map[string]any{
					"public":                false,
					"source_address_prefix": "*",
					"access":                "Deny",
					"direction":             "Inbound",
				},
			},
		},
		Summary: delta.DeltaSummary{AddedNodes: 1},
	}
	r := Evaluate(d)
	if hasReason(r.Reasons, "nsg_allow_all_ingress") {
		t.Fatalf("rule must NOT fire when access is Deny")
	}
}

func TestEvaluate_NSG_OutboundDirection_DoesNotFire(t *testing.T) {
	// Outbound rules are out of scope for this rule -- they govern
	// egress, not ingress, and the "open subnet at network layer"
	// framing only applies to inbound.
	d := delta.Delta{
		AddedNodes: []models.Node{
			{
				ID:           "azurerm_network_security_rule.egress",
				Type:         "access_control",
				ProviderType: "azurerm_network_security_rule",
				Attributes: map[string]any{
					"public":                false,
					"source_address_prefix": "*",
					"access":                "Allow",
					"direction":             "Outbound",
				},
			},
		},
		Summary: delta.DeltaSummary{AddedNodes: 1},
	}
	r := Evaluate(d)
	if hasReason(r.Reasons, "nsg_allow_all_ingress") {
		t.Fatalf("rule must NOT fire on Outbound direction")
	}
}

func TestEvaluate_NSG_UnresolvedAttributes_DoesNotFire(t *testing.T) {
	// Variable-driven values land as missing. Rule MUST stay silent
	// (consistent with master.md design decision 14 -- never guess).
	d := delta.Delta{
		AddedNodes: []models.Node{
			{
				ID:           "azurerm_network_security_rule.unresolved",
				Type:         "access_control",
				ProviderType: "azurerm_network_security_rule",
				Attributes:   map[string]any{"public": false},
			},
		},
		Summary: delta.DeltaSummary{AddedNodes: 1},
	}
	r := Evaluate(d)
	if hasReason(r.Reasons, "nsg_allow_all_ingress") {
		t.Fatalf("rule must NOT fire on unresolved attributes")
	}
}

func TestEvaluate_NSG_AllowAllIngress_CapEnforced(t *testing.T) {
	// phase6CapPerRule = 2: even with three offending rules, only two
	// reasons should be emitted.
	add := func(name string) models.Node {
		return models.Node{
			ID:           "azurerm_network_security_rule." + name,
			Type:         "access_control",
			ProviderType: "azurerm_network_security_rule",
			Attributes: map[string]any{
				"public":                true,
				"source_address_prefix": "*",
				"access":                "Allow",
				"direction":             "Inbound",
			},
		}
	}
	d := delta.Delta{
		AddedNodes: []models.Node{add("a"), add("b"), add("c")},
		Summary:    delta.DeltaSummary{AddedNodes: 3},
	}
	r := Evaluate(d)
	count := 0
	for _, reason := range r.Reasons {
		if reason.RuleID == "nsg_allow_all_ingress" {
			count++
		}
	}
	if count != phase6CapPerRule {
		t.Fatalf("expected exactly %d nsg_allow_all_ingress reasons, got %d", phase6CapPerRule, count)
	}
}

// ---------------------------------------------------------------------------
// storage_account_public
// ---------------------------------------------------------------------------

func TestEvaluate_StorageAccount_PublicNetworkAccess_Fires(t *testing.T) {
	d := delta.Delta{
		AddedNodes: []models.Node{
			{
				ID:           "azurerm_storage_account.public",
				Type:         "storage",
				ProviderType: "azurerm_storage_account",
				Attributes: map[string]any{
					"public":                        true,
					"public_network_access_enabled": true,
				},
			},
		},
		Summary: delta.DeltaSummary{AddedNodes: 1},
	}
	r := Evaluate(d)
	if !hasReason(r.Reasons, "storage_account_public") {
		t.Fatalf("expected storage_account_public, got %+v", r.Reasons)
	}
}

func TestEvaluate_StorageAccount_AllowNestedItemsPublic_Fires(t *testing.T) {
	// allow_nested_items_to_be_public alone is sufficient to expose
	// container contents to anonymous callers.
	d := delta.Delta{
		AddedNodes: []models.Node{
			{
				ID:           "azurerm_storage_account.nested",
				Type:         "storage",
				ProviderType: "azurerm_storage_account",
				Attributes: map[string]any{
					"public":                          true,
					"allow_nested_items_to_be_public": true,
				},
			},
		},
		Summary: delta.DeltaSummary{AddedNodes: 1},
	}
	r := Evaluate(d)
	if !hasReason(r.Reasons, "storage_account_public") {
		t.Fatalf("expected storage_account_public, got %+v", r.Reasons)
	}
}

func TestEvaluate_StorageAccount_PrivateDefaults_DoesNotFire(t *testing.T) {
	d := delta.Delta{
		AddedNodes: []models.Node{
			{
				ID:           "azurerm_storage_account.private",
				Type:         "storage",
				ProviderType: "azurerm_storage_account",
				Attributes: map[string]any{
					"public":                          false,
					"public_network_access_enabled":   false,
					"allow_nested_items_to_be_public": false,
				},
			},
		},
		Summary: delta.DeltaSummary{AddedNodes: 1},
	}
	r := Evaluate(d)
	if hasReason(r.Reasons, "storage_account_public") {
		t.Fatalf("rule must NOT fire on a fully-private storage account")
	}
}

func TestEvaluate_StorageAccount_MissingFlags_DoesNotFire(t *testing.T) {
	// Default-false posture: a missing attribute does NOT imply public
	// access. Variable-driven flags also land as missing and stay silent.
	d := delta.Delta{
		AddedNodes: []models.Node{
			{
				ID:           "azurerm_storage_account.unresolved",
				Type:         "storage",
				ProviderType: "azurerm_storage_account",
				Attributes:   map[string]any{"public": false},
			},
		},
		Summary: delta.DeltaSummary{AddedNodes: 1},
	}
	r := Evaluate(d)
	if hasReason(r.Reasons, "storage_account_public") {
		t.Fatalf("rule must NOT fire on unresolved flags")
	}
}

func TestEvaluate_StorageAccount_CapEnforced(t *testing.T) {
	add := func(name string) models.Node {
		return models.Node{
			ID:           "azurerm_storage_account." + name,
			Type:         "storage",
			ProviderType: "azurerm_storage_account",
			Attributes: map[string]any{
				"public":                        true,
				"public_network_access_enabled": true,
			},
		}
	}
	d := delta.Delta{
		AddedNodes: []models.Node{add("a"), add("b"), add("c")},
		Summary:    delta.DeltaSummary{AddedNodes: 3},
	}
	r := Evaluate(d)
	count := 0
	for _, reason := range r.Reasons {
		if reason.RuleID == "storage_account_public" {
			count++
		}
	}
	if count != phase6CapPerRule {
		t.Fatalf("expected exactly %d storage_account_public reasons, got %d", phase6CapPerRule, count)
	}
}

// ---------------------------------------------------------------------------
// mssql_database_public
// ---------------------------------------------------------------------------

func TestEvaluate_MSSQL_PublicNetworkAccess_Fires(t *testing.T) {
	d := delta.Delta{
		AddedNodes: []models.Node{
			{
				ID:           "azurerm_mssql_server.public",
				Type:         "data",
				ProviderType: "azurerm_mssql_server",
				Attributes: map[string]any{
					"public":                        true,
					"public_network_access_enabled": true,
				},
			},
		},
		Summary: delta.DeltaSummary{AddedNodes: 1},
	}
	r := Evaluate(d)
	if !hasReason(r.Reasons, "mssql_database_public") {
		t.Fatalf("expected mssql_database_public, got %+v", r.Reasons)
	}
}

func TestEvaluate_MSSQL_PrivateServer_DoesNotFire(t *testing.T) {
	d := delta.Delta{
		AddedNodes: []models.Node{
			{
				ID:           "azurerm_mssql_server.private",
				Type:         "data",
				ProviderType: "azurerm_mssql_server",
				Attributes: map[string]any{
					"public":                        false,
					"public_network_access_enabled": false,
				},
			},
		},
		Summary: delta.DeltaSummary{AddedNodes: 1},
	}
	r := Evaluate(d)
	if hasReason(r.Reasons, "mssql_database_public") {
		t.Fatalf("rule must NOT fire on a private MSSQL server")
	}
}

func TestEvaluate_MSSQL_UnresolvedFlag_DoesNotFire(t *testing.T) {
	// Variable-driven flag lands as missing; rule MUST stay silent.
	d := delta.Delta{
		AddedNodes: []models.Node{
			{
				ID:           "azurerm_mssql_server.unresolved",
				Type:         "data",
				ProviderType: "azurerm_mssql_server",
				Attributes:   map[string]any{"public": false},
			},
		},
		Summary: delta.DeltaSummary{AddedNodes: 1},
	}
	r := Evaluate(d)
	if hasReason(r.Reasons, "mssql_database_public") {
		t.Fatalf("rule must NOT fire on unresolved public_network_access_enabled")
	}
}

func TestEvaluate_MSSQL_DatabaseResource_DoesNotFire(t *testing.T) {
	// The rule fires on the SERVER (where the flag lives), never on
	// the database resource itself. A bare azurerm_mssql_database
	// without its parent server in the delta must produce no
	// mssql_database_public reason.
	d := delta.Delta{
		AddedNodes: []models.Node{
			{
				ID:           "azurerm_mssql_database.app",
				Type:         "data",
				ProviderType: "azurerm_mssql_database",
				Attributes: map[string]any{
					"public":                        false,
					"public_network_access_enabled": true,
				},
			},
		},
		Summary: delta.DeltaSummary{AddedNodes: 1},
	}
	r := Evaluate(d)
	if hasReason(r.Reasons, "mssql_database_public") {
		t.Fatalf("rule must fire on the server only, never the database resource")
	}
}

func TestEvaluate_MSSQL_CapEnforced(t *testing.T) {
	add := func(name string) models.Node {
		return models.Node{
			ID:           "azurerm_mssql_server." + name,
			Type:         "data",
			ProviderType: "azurerm_mssql_server",
			Attributes: map[string]any{
				"public":                        true,
				"public_network_access_enabled": true,
			},
		}
	}
	d := delta.Delta{
		AddedNodes: []models.Node{add("a"), add("b"), add("c")},
		Summary:    delta.DeltaSummary{AddedNodes: 3},
	}
	r := Evaluate(d)
	count := 0
	for _, reason := range r.Reasons {
		if reason.RuleID == "mssql_database_public" {
			count++
		}
	}
	if count != phase6CapPerRule {
		t.Fatalf("expected exactly %d mssql_database_public reasons, got %d", phase6CapPerRule, count)
	}
}

// ---------------------------------------------------------------------------
// Conditional-node guard (v1.3 contract): library-mode phantoms must not
// be scored by ANY of the Phase 9 Azure rules.
// ---------------------------------------------------------------------------

func TestEvaluate_AzureRules_ConditionalNodes_NeverScored(t *testing.T) {
	d := delta.Delta{
		AddedNodes: []models.Node{
			{
				ID:           "azurerm_network_security_rule.maybe",
				Type:         "access_control",
				ProviderType: "azurerm_network_security_rule",
				Attributes: map[string]any{
					"public":                true,
					"source_address_prefix": "*",
					"access":                "Allow",
					"direction":             "Inbound",
					"conditional":           true,
				},
			},
			{
				ID:           "azurerm_storage_account.maybe",
				Type:         "storage",
				ProviderType: "azurerm_storage_account",
				Attributes: map[string]any{
					"public":                        true,
					"public_network_access_enabled": true,
					"conditional":                   true,
				},
			},
			{
				ID:           "azurerm_mssql_server.maybe",
				Type:         "data",
				ProviderType: "azurerm_mssql_server",
				Attributes: map[string]any{
					"public":                        true,
					"public_network_access_enabled": true,
					"conditional":                   true,
				},
			},
		},
		Summary: delta.DeltaSummary{AddedNodes: 3},
	}
	r := Evaluate(d)
	for _, reason := range r.Reasons {
		switch reason.RuleID {
		case "nsg_allow_all_ingress",
			"storage_account_public",
			"mssql_database_public":
			t.Errorf("Azure rule %q must not fire on conditional=true node %s",
				reason.RuleID, reason.ResourceID)
		}
	}
}

// ---------------------------------------------------------------------------
// Provider isolation: non-azurerm_* added nodes must never produce any of
// the Phase 9 Azure-rule reasons. This is the structural guarantee that
// AWS-only repos see zero behavior change from v1.3 -> v1.4.
// ---------------------------------------------------------------------------

func TestEvaluate_AzureRules_AWSNodes_NeverFire(t *testing.T) {
	d := delta.Delta{
		AddedNodes: []models.Node{
			{
				ID:           "aws_security_group_rule.open",
				Type:         "access_control",
				ProviderType: "aws_security_group_rule",
				Attributes: map[string]any{
					"public":                true,
					"source_address_prefix": "*",
					"access":                "Allow",
					"direction":             "Inbound",
				},
			},
			{
				ID:           "aws_s3_bucket.public",
				Type:         "storage",
				ProviderType: "aws_s3_bucket",
				Attributes: map[string]any{
					"public":                        true,
					"public_network_access_enabled": true,
				},
			},
			{
				ID:           "aws_db_instance.public",
				Type:         "data",
				ProviderType: "aws_db_instance",
				Attributes: map[string]any{
					"public":                        true,
					"public_network_access_enabled": true,
				},
			},
		},
		Summary: delta.DeltaSummary{AddedNodes: 3},
	}
	r := Evaluate(d)
	for _, reason := range r.Reasons {
		switch reason.RuleID {
		case "nsg_allow_all_ingress",
			"storage_account_public",
			"mssql_database_public":
			t.Errorf("Azure rule %q fired on a non-azurerm node %s -- provider isolation broken",
				reason.RuleID, reason.ResourceID)
		}
	}
}
