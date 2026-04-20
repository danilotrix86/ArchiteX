package risk

import (
	"fmt"

	"architex/delta"
)

// ---------------------------------------------------------------------------
// Phase 8 (v1.3) — Coverage tranche 3 risk rules.
//
// Same per-resource signal philosophy as Phase 6 / Phase 7 PR4: each rule
// reads a small, deterministic property of an added node. No graph
// traversal, no guessing at unresolved expressions. Each rule is capped
// at phase6CapPerRule (2) reasons per evaluation so a sweeping refactor
// cannot single-handedly saturate the 10.0 score cap.
// ---------------------------------------------------------------------------

// Rule 13 — EKS cluster public endpoint.
//
// Triggers for each ADDED aws_eks_cluster whose literal
// `vpc_config.endpoint_public_access` is true AND that does NOT also have
// a literal `vpc_config.endpoint_public_access_cidrs` allow-list. Open
// EKS API endpoints are a notorious lateral-movement primitive: anyone
// on the internet can reach the API server and start hammering RBAC.
//
// Variable-driven `endpoint_public_access = var.public` lands as missing
// in graph.deriveAttributes (only literal bools are promoted) and the
// rule does NOT fire (consistent with master.md design decision 14
// "never guess at unresolved expressions").
//
// Weight 3.5 -- equal to iam_admin_policy_attached. A public EKS API
// without a CIDR allow-list is a textbook ransomware foothold.
func evaluateEKSPublicEndpoint(d delta.Delta) []RiskReason {
	var reasons []RiskReason
	for _, n := range d.AddedNodes {
		if n.ProviderType != "aws_eks_cluster" {
			continue
		}
		if isConditionalNode(n.Attributes) {
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
		if len(reasons) >= phase6CapPerRule {
			break
		}
		reasons = append(reasons, RiskReason{
			RuleID:     "eks_public_endpoint",
			Message:    fmt.Sprintf("EKS cluster %s exposes a public API endpoint with no CIDR allow-list; restrict via vpc_config.endpoint_public_access_cidrs.", n.ID),
			Impact:     "exposure",
			Weight:     3.5,
			ResourceID: n.ID,
		})
	}
	return reasons
}

// Rule 14 — EKS cluster without control-plane logging.
//
// Triggers for each ADDED aws_eks_cluster that has NO literal
// `enabled_cluster_log_types`. Without control-plane logs (api, audit,
// authenticator, controllerManager, scheduler) EKS misuse is invisible
// to detection-and-response: there is no audit trail of who called the
// API server, no record of denied authn/authz attempts, and post-incident
// forensics is impossible.
//
// Weight 1.5 -- this is a hygiene finding, not a blocker. It stacks
// with eks_public_endpoint when both apply (5.0 combined, medium tier).
func evaluateEKSNoLogging(d delta.Delta) []RiskReason {
	var reasons []RiskReason
	for _, n := range d.AddedNodes {
		if n.ProviderType != "aws_eks_cluster" {
			continue
		}
		if isConditionalNode(n.Attributes) {
			continue
		}
		if _, ok := n.Attributes["enabled_cluster_log_types"]; ok {
			continue
		}
		if len(reasons) >= phase6CapPerRule {
			break
		}
		reasons = append(reasons, RiskReason{
			RuleID:     "eks_no_logging",
			Message:    fmt.Sprintf("EKS cluster %s has no enabled_cluster_log_types; control-plane activity will not be auditable.", n.ID),
			Impact:     "observability",
			Weight:     1.5,
			ResourceID: n.ID,
		})
	}
	return reasons
}

// Rule 15 — Autoscaling group with unrestricted scaling.
//
// Triggers for each ADDED aws_autoscaling_group whose literal `max_size`
// exceeds asgUnrestrictedThreshold AND whose `min_size` is missing or
// zero. An ASG that can scale from 0 to 100+ instances on a single
// scaling event is both a runaway-cost vector and a stampede primitive
// (a misconfigured cooldown / health-check can boot 100+ EC2 instances
// in seconds).
//
// Variable-driven `max_size = var.max_capacity` lands as missing and
// the rule does NOT fire.
//
// Weight 1.0 -- low signal on its own. Surfaces as a focus-area item
// for the reviewer; rarely fail-tier on its own.
const asgUnrestrictedThreshold = 100.0

func evaluateASGUnrestrictedScaling(d delta.Delta) []RiskReason {
	var reasons []RiskReason
	for _, n := range d.AddedNodes {
		if n.ProviderType != "aws_autoscaling_group" {
			continue
		}
		if isConditionalNode(n.Attributes) {
			continue
		}
		max, ok := n.Attributes["max_size"].(float64)
		if !ok || max <= asgUnrestrictedThreshold {
			continue
		}
		if v, ok := n.Attributes["min_size"]; ok {
			if min, ok := v.(float64); ok && min > 0 {
				continue
			}
		}
		if len(reasons) >= phase6CapPerRule {
			break
		}
		reasons = append(reasons, RiskReason{
			RuleID:     "asg_unrestricted_scaling",
			Message:    fmt.Sprintf("Autoscaling group %s allows max_size=%d with no min_size floor; a scaling event can launch >100 instances unbounded.", n.ID, int(max)),
			Impact:     "cost",
			Weight:     1.0,
			ResourceID: n.ID,
		})
	}
	return reasons
}

// isConditionalNode is the v1.3 library-mode guard. Phantom resources
// materialized by parser library mode (when `count = var.create ? 1 : 0`
// is the only barrier between "definite" and "absent") carry
// `Attributes["conditional"] = true`. Risk rules MUST treat them as
// non-existent for scoring purposes -- the engine never invents
// findings on resources whose own existence is conditional.
//
// PR2 of v1.3 introduces the conditional marker in the parser; this
// helper lives here because it is the rule layer that owns the
// scoring contract.
func isConditionalNode(attrs map[string]any) bool {
	if attrs == nil {
		return false
	}
	if v, ok := attrs["conditional"]; ok {
		if b, ok := v.(bool); ok && b {
			return true
		}
	}
	return false
}
