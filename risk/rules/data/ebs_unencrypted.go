package data

import (
	"fmt"

	"architex/delta"
	"architex/risk/api"
	"architex/risk/rules/internal/rulefmt"
)

// EBSUnencrypted is the Phase 7 PR4 "ebs_volume_unencrypted" rule.
//
// Triggers for each ADDED aws_ebs_volume whose `encrypted` attribute is
// the LITERAL boolean false. A missing attribute is intentionally NOT a
// match: many providers default to encryption-by-default at the account
// level, and the parser cannot read account state. This means an
// explicit `encrypted = false` (the only way to OPT OUT in Terraform)
// is the only thing that fires. Variable-driven
// `encrypted = var.encrypted` lands as missing and is silent.
//
// Weight 3.0 -- on par with new_entry_point. Unencrypted volumes at
// rest are a regulatory hard-no in many compliance regimes (PCI, HIPAA,
// SOC2), and a single misconfigured EBS attached to a legacy EC2
// silently creates audit findings months later.
var EBSUnencrypted api.Rule = ebsUnencryptedRule{}

type ebsUnencryptedRule struct{}

func (ebsUnencryptedRule) ID() string { return "ebs_volume_unencrypted" }

func (ebsUnencryptedRule) Evaluate(d delta.Delta) []api.RiskReason {
	var reasons []api.RiskReason
	for _, n := range d.AddedNodes {
		if n.ProviderType != "aws_ebs_volume" {
			continue
		}
		if rulefmt.IsConditional(n.Attributes) {
			continue
		}
		v, ok := n.Attributes["encrypted"]
		if !ok {
			continue
		}
		b, ok := v.(bool)
		if !ok || b {
			continue
		}
		if len(reasons) >= rulefmt.DefaultRulePerResourceCap {
			break
		}
		reasons = append(reasons, api.RiskReason{
			RuleID:     "ebs_volume_unencrypted",
			Message:    fmt.Sprintf("EBS volume %s was introduced with encrypted=false; data at rest will be unencrypted.", n.ID),
			Impact:     "data",
			Weight:     3.0,
			ResourceID: n.ID,
		})
	}
	return reasons
}

func (ebsUnencryptedRule) ReviewFocus(reason api.RiskReason, _ delta.Delta) string {
	return fmt.Sprintf(
		"Set encrypted = true on %s; unencrypted volumes at rest fail PCI / HIPAA / SOC2 audits.",
		reason.ResourceID,
	)
}
