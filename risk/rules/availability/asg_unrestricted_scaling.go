// Package availability houses risk rules whose impact is operational
// stability or cost: unbounded autoscaling, missing health checks,
// runaway-cost vectors. These are signals about how the system behaves
// under load or failure, as opposed to its security posture.
//
// Registration of these rules is centralized in the
// architex/risk/rules aggregator.
package availability

import (
	"fmt"

	"architex/delta"
	"architex/risk/api"
	"architex/risk/rules/internal/rulefmt"
)

// asgUnrestrictedThreshold is the max_size value above which an
// autoscaling group is considered "unrestricted". 100 was chosen as the
// pre-refactor v1.3 threshold; see the original PR rationale in
// CHANGELOG.md for v1.3.
const asgUnrestrictedThreshold = 100.0

// ASGUnrestrictedScaling is the Phase 8 (v1.3) "asg_unrestricted_scaling"
// rule.
//
// Triggers for each ADDED aws_autoscaling_group whose literal
// `max_size` exceeds asgUnrestrictedThreshold AND whose `min_size` is
// missing or zero. An ASG that can scale from 0 to 100+ instances on a
// single scaling event is both a runaway-cost vector and a stampede
// primitive (a misconfigured cooldown / health-check can boot 100+ EC2
// instances in seconds).
//
// Variable-driven `max_size = var.max_capacity` lands as missing and
// the rule does NOT fire.
//
// Weight 1.0 -- low signal on its own. Surfaces as a focus-area item
// for the reviewer; rarely fail-tier on its own.
var ASGUnrestrictedScaling api.Rule = asgUnrestrictedScalingRule{}

type asgUnrestrictedScalingRule struct{}

func (asgUnrestrictedScalingRule) ID() string { return "asg_unrestricted_scaling" }

func (asgUnrestrictedScalingRule) Evaluate(d delta.Delta) []api.RiskReason {
	var reasons []api.RiskReason
	for _, n := range d.AddedNodes {
		if n.ProviderType != "aws_autoscaling_group" {
			continue
		}
		if rulefmt.IsConditional(n.Attributes) {
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
		if len(reasons) >= rulefmt.DefaultRulePerResourceCap {
			break
		}
		reasons = append(reasons, api.RiskReason{
			RuleID:     "asg_unrestricted_scaling",
			Message:    fmt.Sprintf("Autoscaling group %s allows max_size=%d with no min_size floor; a scaling event can launch >100 instances unbounded.", n.ID, int(max)),
			Impact:     "cost",
			Weight:     1.0,
			ResourceID: n.ID,
		})
	}
	return reasons
}

func (asgUnrestrictedScalingRule) ReviewFocus(reason api.RiskReason, _ delta.Delta) string {
	return fmt.Sprintf(
		"Cap max_size on %s and set a non-zero min_size floor; an unbounded ASG is both a runaway-cost vector and a stampede primitive.",
		reason.ResourceID,
	)
}
