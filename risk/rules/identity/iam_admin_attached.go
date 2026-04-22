// Package identity houses risk rules that signal a change in who can
// do what -- IAM role attachments, policy grants, principal mappings.
// These rules are domain peers of exposure rules but live in their
// own folder because they answer a different question: "who has
// access?" instead of "what is reachable?".
//
// Registration of these rules is centralized in the
// architex/risk/rules aggregator.
package identity

import (
	"fmt"
	"strings"

	"architex/delta"
	"architex/risk/api"
	"architex/risk/rules/internal/rulefmt"
)

// adminPolicyARNSuffixes lists the AWS-managed policy ARNs whose
// attachment to an IAM role represents an immediate, high-blast-radius
// privilege grant. We match on suffix because the literal string
// "arn:aws:iam::aws:policy/X" is the canonical form a Terraform author
// writes; variable-driven ARNs are captured by the parser as nil and
// intentionally do not match (we do not guess at unresolved
// expressions).
var adminPolicyARNSuffixes = []string{
	":policy/AdministratorAccess",
	":policy/IAMFullAccess",
}

// IAMAdminAttached is the Phase 6 "iam_admin_policy_attached" rule.
//
// Triggers when an aws_iam_role_policy_attachment is ADDED whose
// policy_arn literal ends in a known wildcard-admin AWS managed policy.
// Weight 3.5 reflects the privilege-escalation impact: an attacker who
// later compromises any principal assuming this role has root-equivalent
// access.
var IAMAdminAttached api.Rule = iamAdminAttachedRule{}

type iamAdminAttachedRule struct{}

func (iamAdminAttachedRule) ID() string { return "iam_admin_policy_attached" }

func (iamAdminAttachedRule) Evaluate(d delta.Delta) []api.RiskReason {
	var reasons []api.RiskReason

	for _, n := range d.AddedNodes {
		if n.ProviderType != "aws_iam_role_policy_attachment" {
			continue
		}
		if rulefmt.IsConditional(n.Attributes) {
			continue
		}
		raw, ok := n.Attributes["policy_arn"]
		if !ok {
			continue
		}
		arn, ok := raw.(string)
		if !ok || arn == "" {
			continue
		}
		matchedSuffix := ""
		for _, suffix := range adminPolicyARNSuffixes {
			if strings.HasSuffix(arn, suffix) {
				matchedSuffix = strings.TrimPrefix(suffix, ":policy/")
				break
			}
		}
		if matchedSuffix == "" {
			continue
		}
		if len(reasons) >= rulefmt.DefaultRulePerResourceCap {
			break
		}
		reasons = append(reasons, api.RiskReason{
			RuleID:     "iam_admin_policy_attached",
			Message:    fmt.Sprintf("IAM attachment %s grants %s; review for privilege escalation.", n.ID, matchedSuffix),
			Impact:     "identity",
			Weight:     3.5,
			ResourceID: n.ID,
		})
	}

	return reasons
}

func (iamAdminAttachedRule) ReviewFocus(reason api.RiskReason, _ delta.Delta) string {
	return fmt.Sprintf(
		"Audit the privilege grant on %s -- AdministratorAccess / IAMFullAccess gives root-equivalent control to anyone who assumes the role.",
		reason.ResourceID,
	)
}
