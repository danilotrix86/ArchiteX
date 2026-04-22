package data

import (
	"fmt"

	"architex/delta"
	"architex/risk/api"
	"architex/risk/rules/internal/rulefmt"
)

// MSSQLDatabasePublic is the Phase 9 (v1.4) "mssql_database_public"
// rule.
//
// Triggers when an azurerm_mssql_server is ADDED with the literal flag
// `public_network_access_enabled = true`. The fired reason is attached
// to the server resource (its public-network-access boundary), not the
// per-database resource -- matching where the configuration actually
// lives in Terraform.
//
// CAVEAT: this rule does NOT cross-check whether an
// azurerm_mssql_firewall_rule exists to scope the public access. That
// would require graph traversal, which Phase 9 deliberately defers to
// a later tranche to keep the per-resource signal pattern intact (same
// reasoning as s3_bucket_public_exposure, which does not check whether
// a separate aws_s3_bucket_policy denies public read). A reviewer who
// has scoped public access via firewall rules can suppress this
// finding in `.architex.yml` or via inline `# architex:ignore=`.
//
// Variable-driven flags (e.g.
// `public_network_access_enabled = var.public`) land as missing in
// graph.deriveAttributes and the rule does NOT fire.
//
// Weight 3.5 -- equal to nsg_allow_all_ingress. A managed-database
// public endpoint without firewall scoping is a textbook
// data-exfiltration surface and historically the root cause of
// multiple high-profile breaches.
var MSSQLDatabasePublic api.Rule = mssqlDatabasePublicRule{}

type mssqlDatabasePublicRule struct{}

func (mssqlDatabasePublicRule) ID() string { return "mssql_database_public" }

func (mssqlDatabasePublicRule) Evaluate(d delta.Delta) []api.RiskReason {
	var reasons []api.RiskReason
	for _, n := range d.AddedNodes {
		if n.ProviderType != "azurerm_mssql_server" {
			continue
		}
		if rulefmt.IsConditional(n.Attributes) {
			continue
		}
		open, ok := n.Attributes["public_network_access_enabled"].(bool)
		if !ok || !open {
			continue
		}
		if len(reasons) >= rulefmt.DefaultRulePerResourceCap {
			break
		}
		reasons = append(reasons, api.RiskReason{
			RuleID:     "mssql_database_public",
			Message:    fmt.Sprintf("MSSQL server %s has public_network_access_enabled=true; managed databases on this server are reachable from the public internet (verify a firewall rule scopes access).", n.ID),
			Impact:     "data_exposure",
			Weight:     3.5,
			ResourceID: n.ID,
		})
	}
	return reasons
}

func (mssqlDatabasePublicRule) ReviewFocus(reason api.RiskReason, _ delta.Delta) string {
	return fmt.Sprintf(
		"Restrict public access on MSSQL server %s -- set public_network_access_enabled = false, or scope reachability via an azurerm_mssql_firewall_rule with explicit IP ranges.",
		reason.ResourceID,
	)
}
