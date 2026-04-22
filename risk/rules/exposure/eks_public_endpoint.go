package exposure

import (
	"fmt"

	"architex/delta"
	"architex/risk/api"
	"architex/risk/rules/internal/rulefmt"
)

// EKSPublicEndpoint is the Phase 8 (v1.3) "eks_public_endpoint" rule.
//
// Triggers for each ADDED aws_eks_cluster whose literal
// `vpc_config.endpoint_public_access` is true AND that does NOT also
// have a literal `vpc_config.endpoint_public_access_cidrs` allow-list.
// Open EKS API endpoints are a notorious lateral-movement primitive:
// anyone on the internet can reach the API server and start hammering
// RBAC.
//
// Variable-driven `endpoint_public_access = var.public` lands as
// missing in graph.deriveAttributes (only literal bools are promoted)
// and the rule does NOT fire (consistent with master.md design
// decision 14, "never guess at unresolved expressions").
//
// Weight 3.5 -- equal to iam_admin_policy_attached. A public EKS API
// without a CIDR allow-list is a textbook ransomware foothold.
var EKSPublicEndpoint api.Rule = eksPublicEndpointRule{}

type eksPublicEndpointRule struct{}

func (eksPublicEndpointRule) ID() string { return "eks_public_endpoint" }

func (eksPublicEndpointRule) Evaluate(d delta.Delta) []api.RiskReason {
	var reasons []api.RiskReason
	for _, n := range d.AddedNodes {
		if n.ProviderType != "aws_eks_cluster" {
			continue
		}
		if rulefmt.IsConditional(n.Attributes) {
			continue
		}
		open, ok := n.Attributes["endpoint_public_access"].(bool)
		if !ok || !open {
			continue
		}
		if _, ok := n.Attributes["endpoint_public_access_cidrs"]; ok {
			// CIDR allow-list present -- the endpoint is public but
			// scoped. Trust the reviewer's allow-list.
			continue
		}
		if len(reasons) >= rulefmt.DefaultRulePerResourceCap {
			break
		}
		reasons = append(reasons, api.RiskReason{
			RuleID:     "eks_public_endpoint",
			Message:    fmt.Sprintf("EKS cluster %s exposes a public API endpoint with no CIDR allow-list; restrict via vpc_config.endpoint_public_access_cidrs.", n.ID),
			Impact:     "exposure",
			Weight:     3.5,
			ResourceID: n.ID,
		})
	}
	return reasons
}

func (eksPublicEndpointRule) ReviewFocus(reason api.RiskReason, _ delta.Delta) string {
	return fmt.Sprintf(
		"Restrict the EKS API endpoint on %s via vpc_config.endpoint_public_access_cidrs, or set endpoint_public_access = false and access through a private link.",
		reason.ResourceID,
	)
}
