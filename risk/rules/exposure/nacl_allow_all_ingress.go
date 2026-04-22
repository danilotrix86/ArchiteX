package exposure

import (
	"fmt"
	"strings"

	"architex/delta"
	"architex/risk/api"
	"architex/risk/rules/internal/rulefmt"
)

// NACLAllowAllIngress is the Phase 7 PR4 "nacl_allow_all_ingress" rule.
//
// Triggers when an aws_network_acl_rule is ADDED with the trio
// (cidr_block = 0.0.0.0/0, egress = false, rule_action = "allow"). NACLs
// are ordered, so even if a later rule denies traffic, an Allow with
// 0.0.0.0/0 sitting at a low rule_number will be evaluated first. Any
// ONE such rule effectively opens the subnet. Reviewers should justify
// it explicitly (e.g. "yes, this NACL fronts a public ALB").
//
// Weight 3.5 -- equal to iam_admin_policy_attached. NACLs sit a layer
// below SG and are easy to misconfigure (rule_number ordering); a
// permissive Allow at a low number quietly defeats the SG above it.
//
// Variable-driven attributes land as missing and the rule does not
// fire. Egress rules (`egress = true`) are treated as an outbound
// concern and are out of scope -- they do not match.
var NACLAllowAllIngress api.Rule = naclAllowAllIngressRule{}

type naclAllowAllIngressRule struct{}

func (naclAllowAllIngressRule) ID() string { return "nacl_allow_all_ingress" }

func (naclAllowAllIngressRule) Evaluate(d delta.Delta) []api.RiskReason {
	var reasons []api.RiskReason
	for _, n := range d.AddedNodes {
		if n.ProviderType != "aws_network_acl_rule" {
			continue
		}
		if rulefmt.IsConditional(n.Attributes) {
			continue
		}
		cidr, _ := n.Attributes["cidr_block"].(string)
		if cidr != "0.0.0.0/0" {
			continue
		}
		action, _ := n.Attributes["rule_action"].(string)
		if !strings.EqualFold(action, "allow") {
			continue
		}
		if egress, ok := n.Attributes["egress"].(bool); ok && egress {
			continue
		}
		if len(reasons) >= rulefmt.DefaultRulePerResourceCap {
			break
		}
		reasons = append(reasons, api.RiskReason{
			RuleID:     "nacl_allow_all_ingress",
			Message:    fmt.Sprintf("Network ACL rule %s allows inbound traffic from 0.0.0.0/0; the subnet is open at the network layer.", n.ID),
			Impact:     "exposure",
			Weight:     3.5,
			ResourceID: n.ID,
		})
	}
	return reasons
}

func (naclAllowAllIngressRule) ReviewFocus(reason api.RiskReason, _ delta.Delta) string {
	return fmt.Sprintf(
		"Tighten the NACL rule %s; an Allow on 0.0.0.0/0 at a low rule_number defeats any tighter SG above it.",
		reason.ResourceID,
	)
}
