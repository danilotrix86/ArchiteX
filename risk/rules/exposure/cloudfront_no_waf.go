package exposure

import (
	"fmt"

	"architex/delta"
	"architex/risk/api"
	"architex/risk/rules/internal/rulefmt"
)

// CloudFrontNoWAF is the Phase 7 PR4 "cloudfront_no_waf" rule.
//
// Triggers for each ADDED aws_cloudfront_distribution that does NOT
// have a literal `web_acl_id` attribute. CF distros are internet-facing
// edge caches; without a WAF they expose the origin to every L7 attack
// pattern AWS WAF would otherwise mitigate.
//
// Variable-driven `web_acl_id = var.waf_id` lands here as missing (the
// graph layer only promotes literals) and the rule fires conservatively.
// This is consistent with master.md design decision 14 ("never guess at
// unresolved expressions"). A reviewer who sees this finding on a
// var-driven attachment can suppress it in `.architex.yml`.
//
// Weight 2.5 -- below the existing entry_point rule (3.0) and the AWS
// Top-10 group (3.0-4.0). It is signal, not blocker, on its own; it
// stacks with new_entry_point so a brand-new public CF distro without
// WAF lands at 5.5 (medium).
var CloudFrontNoWAF api.Rule = cloudfrontNoWAFRule{}

type cloudfrontNoWAFRule struct{}

func (cloudfrontNoWAFRule) ID() string { return "cloudfront_no_waf" }

func (cloudfrontNoWAFRule) Evaluate(d delta.Delta) []api.RiskReason {
	var reasons []api.RiskReason
	for _, n := range d.AddedNodes {
		if n.ProviderType != "aws_cloudfront_distribution" {
			continue
		}
		if rulefmt.IsConditional(n.Attributes) {
			continue
		}
		if v, ok := n.Attributes["web_acl_id"]; ok {
			if s, ok := v.(string); ok && s != "" {
				continue
			}
		}
		if len(reasons) >= rulefmt.DefaultRulePerResourceCap {
			break
		}
		reasons = append(reasons, api.RiskReason{
			RuleID:     "cloudfront_no_waf",
			Message:    fmt.Sprintf("CloudFront distribution %s was introduced without a WAF (web_acl_id); add AWS WAF to mitigate L7 attacks at the edge.", n.ID),
			Impact:     "exposure",
			Weight:     2.5,
			ResourceID: n.ID,
		})
	}
	return reasons
}

func (cloudfrontNoWAFRule) ReviewFocus(reason api.RiskReason, _ delta.Delta) string {
	return fmt.Sprintf(
		"Add an AWS WAF (web_acl_id) to %s before merging; CloudFront edges without a WAF expose the origin to every L7 attack pattern.",
		reason.ResourceID,
	)
}
