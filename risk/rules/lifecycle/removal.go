// Package lifecycle houses risk rules about the lifecycle of
// resources -- creation, removal, replacement -- as opposed to their
// security posture. Today this is just resource_removed; future
// rules about destroy/replace plans will land here.
//
// Registration of these rules is centralized in the architex/risk/rules
// aggregator; see the package comment in
// architex/risk/rules/exposure/public_exposure.go for the rationale.
package lifecycle

import (
	"fmt"

	"architex/delta"
	"architex/risk/api"
	"architex/risk/rules/internal/rulefmt"
)

// removalMaxReasons bounds how many resource_removed findings the rule
// emits in one delta. The cap (2 reasons => 1.0 total weight) prevents
// large teardowns from dominating the score; a reviewer who sees two
// instances of resource_removed already understands the pattern. Pre-
// refactor this constant lived in risk/rules.go as a private
// package-level. Same value (2) carries over verbatim.
const removalMaxReasons = 2

// Removal is the v1.0 "resource_removed" rule.
//
// Triggers up to removalMaxReasons times, once per RemovedNode in the
// delta, in iteration order. Weight 0.5 per finding -- intentionally
// low because removal is often intentional cleanup, but always worth
// surfacing so reviewers notice unintended drift.
var Removal api.Rule = removalRule{}

type removalRule struct{}

func (removalRule) ID() string { return "resource_removed" }

func (removalRule) Evaluate(d delta.Delta) []api.RiskReason {
	var reasons []api.RiskReason
	for _, n := range d.RemovedNodes {
		if len(reasons) >= removalMaxReasons {
			break
		}
		reasons = append(reasons, api.RiskReason{
			RuleID:     "resource_removed",
			Message:    fmt.Sprintf("Resource %s was removed.", n.ID),
			Impact:     "change",
			Weight:     0.5,
			ResourceID: n.ID,
		})
	}
	return reasons
}

// ReviewFocus aggregates every removed node ID into one bullet.
// Interpreter dedupe collapses repeated returns into a single bullet
// even if removalMaxReasons fired multiple findings.
func (removalRule) ReviewFocus(reason api.RiskReason, d delta.Delta) string {
	ids := rulefmt.RemovedNodeIDs(d)
	return fmt.Sprintf(
		"Confirm the removal of %s is intended; check for orphaned dependencies and audit log retention.",
		rulefmt.JoinHumanList(ids),
	)
}
