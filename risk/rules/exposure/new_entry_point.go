package exposure

import (
	"fmt"

	"architex/delta"
	"architex/risk/api"
	"architex/risk/rules/internal/rulefmt"
)

// NewEntryPoint is the v1.0 "new_entry_point" rule.
//
// Triggers for each ADDED node whose abstract type is "entry_point"
// (load balancers, API Gateway stages, function URLs, etc.). Weight
// 3.0: a new entry point is the single most common precursor to a
// production incident, but it is not as immediately dangerous as a
// resource flipping from private to public (which weights 4.0).
var NewEntryPoint api.Rule = newEntryPointRule{}

type newEntryPointRule struct{}

func (newEntryPointRule) ID() string { return "new_entry_point" }

func (newEntryPointRule) Evaluate(d delta.Delta) []api.RiskReason {
	var reasons []api.RiskReason
	for _, n := range d.AddedNodes {
		if n.Type == "entry_point" {
			reasons = append(reasons, api.RiskReason{
				RuleID:     "new_entry_point",
				Message:    fmt.Sprintf("New public entry point %s introduced.", n.ID),
				Impact:     "exposure",
				Weight:     3.0,
				ResourceID: n.ID,
			})
		}
	}
	return reasons
}

// ReviewFocus aggregates every newly-added entry_point node into a
// single bullet. Interpreter dedupe means one bullet per rule even when
// multiple entry points are added in the same delta.
func (newEntryPointRule) ReviewFocus(reason api.RiskReason, d delta.Delta) string {
	ids := rulefmt.AddedNodeIDsByType(d, "entry_point")
	return fmt.Sprintf(
		"Review the new entry point(s) %s for TLS, authentication, and exposure scope.",
		rulefmt.JoinHumanList(ids),
	)
}
