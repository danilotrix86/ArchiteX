package exposure

import (
	"fmt"
	"strings"

	"architex/delta"
	"architex/risk/api"
	"architex/risk/rules/internal/rulefmt"
)

// NSGAllowAllIngress is the Phase 9 (v1.4) "nsg_allow_all_ingress"
// rule -- the Azure analog of nacl_allow_all_ingress.
//
// Triggers when an azurerm_network_security_rule is ADDED with the
// literal trio (source_address_prefix in {"*", "0.0.0.0/0"},
// access = "Allow", direction = "Inbound"). Any one such rule can
// silently open the subnet at the network layer, regardless of how
// restrictive the rules above and below it look. Reviewers should
// justify it explicitly.
//
// Variable-driven attributes (e.g. `source_address_prefix = var.cidr`)
// land as missing in graph.deriveAttributes (only literal strings are
// promoted) and the rule does NOT fire -- consistent with master.md
// design decision 14 ("never guess at unresolved expressions").
//
// Weight 3.5 -- equal to nacl_allow_all_ingress and
// iam_admin_policy_attached. A `*` ingress rule is the Azure equivalent
// of an AWS NACL or SG opened to 0.0.0.0/0 and carries the same
// blast-radius implications. The rule lives next to its AWS counterpart
// (in package exposure, not a hypothetical azure/ package) because the
// readability refactor organizes rules by *what they signal*, not by
// *which provider they target*.
var NSGAllowAllIngress api.Rule = nsgAllowAllIngressRule{}

type nsgAllowAllIngressRule struct{}

func (nsgAllowAllIngressRule) ID() string { return "nsg_allow_all_ingress" }

func (nsgAllowAllIngressRule) Evaluate(d delta.Delta) []api.RiskReason {
	var reasons []api.RiskReason
	for _, n := range d.AddedNodes {
		if n.ProviderType != "azurerm_network_security_rule" {
			continue
		}
		if rulefmt.IsConditional(n.Attributes) {
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
		if len(reasons) >= rulefmt.DefaultRulePerResourceCap {
			break
		}
		reasons = append(reasons, api.RiskReason{
			RuleID:     "nsg_allow_all_ingress",
			Message:    fmt.Sprintf("Network security rule %s allows inbound traffic from %s; the subnet is open at the network layer.", n.ID, src),
			Impact:     "exposure",
			Weight:     3.5,
			ResourceID: n.ID,
		})
	}
	return reasons
}

func (nsgAllowAllIngressRule) ReviewFocus(reason api.RiskReason, _ delta.Delta) string {
	return fmt.Sprintf(
		"Tighten the NSG rule %s; an Allow inbound from \"*\" / 0.0.0.0/0 opens the subnet at the network layer regardless of any tighter NSG rule above it.",
		reason.ResourceID,
	)
}
