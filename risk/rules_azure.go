package risk

import (
	"fmt"
	"strings"

	"architex/delta"
)

// ---------------------------------------------------------------------------
// Phase 9 (v1.4) — Azure tranche-0 risk rules.
//
// Same per-resource signal philosophy as the AWS-side Phase 6 / Phase 7 PR4
// / Phase 8 rules: each rule reads a small, deterministic property of an
// added node. No graph traversal, no guessing at unresolved expressions.
// Each rule is capped at phase6CapPerRule (2) reasons per evaluation so a
// sweeping refactor cannot single-handedly saturate the 10.0 score cap.
//
// All three rules respect the v1.3 `isConditionalNode` library-mode guard;
// resources whose existence depends on a `count = var.create ? 1 : 0`
// pattern are treated as non-existent for scoring purposes.
// ---------------------------------------------------------------------------

// Rule 16 — Azure NSG rule allows world-inbound (Azure analog of
// nacl_allow_all_ingress).
//
// Triggers when an azurerm_network_security_rule is ADDED with the literal
// trio (source_address_prefix in {"*", "0.0.0.0/0"}, access = "Allow",
// direction = "Inbound"). Any one such rule can silently open the subnet
// at the network layer, regardless of how restrictive the rules above and
// below it look. Reviewers should justify it explicitly.
//
// Variable-driven attributes (e.g. `source_address_prefix = var.cidr`)
// land as missing in graph.deriveAttributes (only literal strings are
// promoted) and the rule does NOT fire -- consistent with master.md
// design decision 14 ("never guess at unresolved expressions").
//
// Weight 3.5 -- equal to nacl_allow_all_ingress and iam_admin_policy_attached.
// A `*` ingress rule is the Azure equivalent of an AWS NACL or SG opened
// to 0.0.0.0/0 and carries the same blast-radius implications.
func evaluateNSGAllowAllIngress(d delta.Delta) []RiskReason {
	var reasons []RiskReason
	for _, n := range d.AddedNodes {
		if n.ProviderType != "azurerm_network_security_rule" {
			continue
		}
		if isConditionalNode(n.Attributes) {
			continue
		}
		src, _ := n.Attributes["source_address_prefix"].(string)
		if src != "*" && src != "0.0.0.0/0" {
			continue
		}
		access, _ := n.Attributes["access"].(string)
		if !strings.EqualFold(access, "Allow") {
			continue
		}
		dir, _ := n.Attributes["direction"].(string)
		if !strings.EqualFold(dir, "Inbound") {
			continue
		}
		if len(reasons) >= phase6CapPerRule {
			break
		}
		reasons = append(reasons, RiskReason{
			RuleID:     "nsg_allow_all_ingress",
			Message:    fmt.Sprintf("Network security rule %s allows inbound traffic from %s; the subnet is open at the network layer.", n.ID, src),
			Impact:     "exposure",
			Weight:     3.5,
			ResourceID: n.ID,
		})
	}
	return reasons
}

// Rule 17 — Azure storage account public (Azure analog of
// s3_bucket_public_exposure).
//
// Triggers when an azurerm_storage_account is ADDED with EITHER
// `public_network_access_enabled = true` OR `allow_nested_items_to_be_public
// = true`. Either flag is sufficient to expose blob/file/table/queue
// contents to anonymous internet callers (the public-network-access flag
// opens the data plane; allow-nested-items opens per-container anonymous
// access).
//
// Default-false posture: a missing attribute does NOT fire. azurerm
// provider defaults vary by version, and we never guess at runtime
// state. Variable-driven flags also land as missing and stay silent.
//
// Weight 4.0 -- equal to s3_bucket_public_exposure. A public storage
// account is the Azure equivalent of a public S3 bucket and carries the
// same exfiltration risk. The two literal flags are independent ways
// the same posture is configured; we report the rule once per resource
// (not once per flag) to avoid double-counting.
func evaluateStorageAccountPublic(d delta.Delta) []RiskReason {
	var reasons []RiskReason
	for _, n := range d.AddedNodes {
		if n.ProviderType != "azurerm_storage_account" {
			continue
		}
		if isConditionalNode(n.Attributes) {
			continue
		}
		var msg string
		if b, ok := n.Attributes["public_network_access_enabled"].(bool); ok && b {
			msg = fmt.Sprintf("Storage account %s has public_network_access_enabled=true; the data plane is reachable from the public internet.", n.ID)
		} else if b, ok := n.Attributes["allow_nested_items_to_be_public"].(bool); ok && b {
			msg = fmt.Sprintf("Storage account %s has allow_nested_items_to_be_public=true; container-level anonymous access is permitted.", n.ID)
		} else {
			continue
		}
		if len(reasons) >= phase6CapPerRule {
			break
		}
		reasons = append(reasons, RiskReason{
			RuleID:     "storage_account_public",
			Message:    msg,
			Impact:     "exposure",
			Weight:     4.0,
			ResourceID: n.ID,
		})
	}
	return reasons
}

// Rule 18 — Azure MSSQL server reachable from the public internet.
//
// Triggers when an azurerm_mssql_server is ADDED with the literal flag
// `public_network_access_enabled = true`. The fired reason is attached
// to the server resource (its public-network-access boundary), not the
// per-database resource -- matching where the configuration actually
// lives in Terraform.
//
// CAVEAT: this rule does NOT cross-check whether an
// azurerm_mssql_firewall_rule exists to scope the public access. That
// would require graph traversal, which Phase 9 deliberately defers to a
// later tranche to keep the per-resource signal pattern intact (same
// reasoning as s3_bucket_public_exposure, which does not check whether
// a separate aws_s3_bucket_policy denies public read). A reviewer who
// has scoped public access via firewall rules can suppress this finding
// in `.architex.yml` (Phase 7 PR3) or via inline `# architex:ignore=`.
//
// Variable-driven flags (e.g. `public_network_access_enabled = var.public`)
// land as missing in graph.deriveAttributes and the rule does NOT fire.
//
// Weight 3.5 -- equal to nsg_allow_all_ingress. A managed-database
// public endpoint without firewall scoping is a textbook data-exfiltration
// surface and historically the root cause of multiple high-profile breaches.
func evaluateMSSQLDatabasePublic(d delta.Delta) []RiskReason {
	var reasons []RiskReason
	for _, n := range d.AddedNodes {
		if n.ProviderType != "azurerm_mssql_server" {
			continue
		}
		if isConditionalNode(n.Attributes) {
			continue
		}
		open, ok := n.Attributes["public_network_access_enabled"].(bool)
		if !ok || !open {
			continue
		}
		if len(reasons) >= phase6CapPerRule {
			break
		}
		reasons = append(reasons, RiskReason{
			RuleID:     "mssql_database_public",
			Message:    fmt.Sprintf("MSSQL server %s has public_network_access_enabled=true; managed databases on this server are reachable from the public internet (verify a firewall rule scopes access).", n.ID),
			Impact:     "data_exposure",
			Weight:     3.5,
			ResourceID: n.ID,
		})
	}
	return reasons
}
