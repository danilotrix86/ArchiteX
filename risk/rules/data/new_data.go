// Package data houses risk rules that signal a change to where data
// lives in the architecture (a new database appearing, an existing data
// store potentially gaining public reach, etc.). Each file owns exactly
// one rule plus its reviewer-facing copy.
//
// Registration of these rules is centralized in the architex/risk/rules
// aggregator; see the package comment in
// architex/risk/rules/exposure/public_exposure.go for the rationale.
package data

import (
	"fmt"

	"architex/delta"
	"architex/risk/api"
	"architex/risk/rules/internal/rulefmt"
)

// NewData is the v1.0 "new_data_resource" rule.
//
// Triggers for each ADDED node whose abstract type is "data" (RDS
// instances, DynamoDB tables, S3 buckets, etc.). Weight 2.5: lower than
// new_entry_point (3.0) because a new data store does not by itself
// expose anything, but high enough that a reviewer always sees it.
var NewData api.Rule = newDataRule{}

type newDataRule struct{}

func (newDataRule) ID() string { return "new_data_resource" }

func (newDataRule) Evaluate(d delta.Delta) []api.RiskReason {
	var reasons []api.RiskReason
	for _, n := range d.AddedNodes {
		if n.Type == "data" {
			reasons = append(reasons, api.RiskReason{
				RuleID:     "new_data_resource",
				Message:    fmt.Sprintf("New data resource %s introduced.", n.ID),
				Impact:     "data",
				Weight:     2.5,
				ResourceID: n.ID,
			})
		}
	}
	return reasons
}

// ReviewFocus aggregates every newly-added data node into a single
// bullet (interpreter dedupe collapses repeated returns into one).
func (newDataRule) ReviewFocus(reason api.RiskReason, d delta.Delta) string {
	ids := rulefmt.AddedNodeIDsByType(d, "data")
	return fmt.Sprintf(
		"Verify encryption at rest, backups, and access controls on the new data resource(s): %s.",
		rulefmt.JoinHumanList(ids),
	)
}
