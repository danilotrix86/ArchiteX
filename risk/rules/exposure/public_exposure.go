// Package exposure houses risk rules that signal a change to the
// publicly-reachable surface of the architecture (a resource becoming
// internet-routable, a new public entry point appearing, etc.). Each
// file in this package owns exactly one rule plus its reviewer-facing
// "Suggested Review Focus" copy.
//
// Rules in this package register themselves with architex/risk/api
// indirectly: they are exported as values from this package and the
// architex/risk/rules aggregator package wires them into the registry
// in a deterministic order. Keeping registration centralized (rather
// than per-file init() calls) keeps the cross-package init order stable
// and easy to read in one place.
package exposure

import (
	"fmt"

	"architex/delta"
	"architex/risk/api"
	"architex/risk/rules/internal/rulefmt"
)

// PublicExposure is the v1.0 "public_exposure_introduced" rule.
//
// Triggers when a node's "public" attribute transitioned from false to
// true between base and head. Weight 4.0 -- the highest single-rule
// weight in the v1.0 set -- because internet exposure is a step-change
// in attack surface that almost always warrants human review.
//
// The rule is per-resource: every changed node that flipped public
// emits its own RiskReason carrying ResourceID. This is what lets
// .architex.yml suppressions target individual resources instead of
// silencing the rule globally.
var PublicExposure api.Rule = publicExposureRule{}

type publicExposureRule struct{}

func (publicExposureRule) ID() string { return "public_exposure_introduced" }

func (publicExposureRule) Evaluate(d delta.Delta) []api.RiskReason {
	var reasons []api.RiskReason
	for _, cn := range d.ChangedNodes {
		attr, ok := cn.ChangedAttributes["public"]
		if !ok {
			continue
		}
		// Defensive type assertion: ChangedAttribute fields are `any`,
		// so a future serialization round-trip could produce non-bool
		// values. Skip rather than panic; missing data should never
		// false-fire a high-weight rule.
		before, beforeOK := attr.Before.(bool)
		after, afterOK := attr.After.(bool)
		if !beforeOK || !afterOK {
			continue
		}
		if !before && after {
			reasons = append(reasons, api.RiskReason{
				RuleID:     "public_exposure_introduced",
				Message:    fmt.Sprintf("Resource %s became publicly accessible.", cn.ID),
				Impact:     "exposure",
				Weight:     4.0,
				ResourceID: cn.ID,
			})
		}
	}
	return reasons
}

// ReviewFocus aggregates EVERY changed node whose `public` attribute
// flipped into a single bullet. The interpreter dedupes on the returned
// string, so emitting the same aggregate string per RiskReason produces
// exactly one bullet per rule -- matching the pre-refactor
// interpreter.focusForRule output byte-for-byte.
func (publicExposureRule) ReviewFocus(reason api.RiskReason, d delta.Delta) string {
	ids := rulefmt.ChangedNodeIDsByAttr(d, "public")
	return fmt.Sprintf(
		"Confirm that public exposure is intended on %s and that ingress is restricted to required ports and sources.",
		rulefmt.JoinHumanList(ids),
	)
}
