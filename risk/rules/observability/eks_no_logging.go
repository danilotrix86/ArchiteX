// Package observability houses risk rules whose impact is the loss
// (or absence) of telemetry: missing audit trails, disabled flow logs,
// no control-plane logging. These rules are signals about the ability
// to *detect* a problem rather than the existence of one.
//
// Registration of these rules is centralized in the
// architex/risk/rules aggregator.
package observability

import (
	"fmt"

	"architex/delta"
	"architex/risk/api"
	"architex/risk/rules/internal/rulefmt"
)

// EKSNoLogging is the Phase 8 (v1.3) "eks_no_logging" rule.
//
// Triggers for each ADDED aws_eks_cluster that has NO literal
// `enabled_cluster_log_types`. Without control-plane logs (api, audit,
// authenticator, controllerManager, scheduler) EKS misuse is invisible
// to detection-and-response: there is no audit trail of who called the
// API server, no record of denied authn/authz attempts, and
// post-incident forensics is impossible.
//
// Weight 1.5 -- this is a hygiene finding, not a blocker. It stacks
// with eks_public_endpoint when both apply (5.0 combined, medium tier).
var EKSNoLogging api.Rule = eksNoLoggingRule{}

type eksNoLoggingRule struct{}

func (eksNoLoggingRule) ID() string { return "eks_no_logging" }

func (eksNoLoggingRule) Evaluate(d delta.Delta) []api.RiskReason {
	var reasons []api.RiskReason
	for _, n := range d.AddedNodes {
		if n.ProviderType != "aws_eks_cluster" {
			continue
		}
		if rulefmt.IsConditional(n.Attributes) {
			continue
		}
		if _, ok := n.Attributes["enabled_cluster_log_types"]; ok {
			continue
		}
		if len(reasons) >= rulefmt.DefaultRulePerResourceCap {
			break
		}
		reasons = append(reasons, api.RiskReason{
			RuleID:     "eks_no_logging",
			Message:    fmt.Sprintf("EKS cluster %s has no enabled_cluster_log_types; control-plane activity will not be auditable.", n.ID),
			Impact:     "observability",
			Weight:     1.5,
			ResourceID: n.ID,
		})
	}
	return reasons
}

func (eksNoLoggingRule) ReviewFocus(reason api.RiskReason, _ delta.Delta) string {
	return fmt.Sprintf(
		"Enable enabled_cluster_log_types on %s (api, audit, authenticator are the bare minimum) so control-plane activity is forensically auditable.",
		reason.ResourceID,
	)
}
