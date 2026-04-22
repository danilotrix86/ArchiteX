package data

import (
	"architex/delta"
	"architex/risk/api"
)

// PotentialDataExposure is the v1.0 "potential_data_exposure" rule.
//
// This is the only v1.0 rule with a cross-rule precondition: it fires
// only when public_exposure_introduced would also fire AND either:
//
//   - a new data node was added in the same delta, OR
//   - an existing access_control or data node changed in the same delta.
//
// Pre-refactor the orchestrator passed a boolean ("did public exposure
// trigger?") into evaluateDataExposure. Post-refactor every rule's
// Evaluate takes only the delta -- so this rule recomputes its own
// precondition from the delta directly. The check is identical to
// PublicExposure.Evaluate's filter, scoped to "is there ANY hit?", and
// is therefore guaranteed to agree with the public-exposure rule
// regardless of registration order.
//
// Weight 2.0: cross-resource warning, lower than the per-resource
// public_exposure_introduced (4.0) it depends on. The rule is
// intentionally not per-resource (ResourceID is empty) -- it's a
// posture warning, not a finding against one resource.
var PotentialDataExposure api.Rule = potentialDataExposureRule{}

type potentialDataExposureRule struct{}

func (potentialDataExposureRule) ID() string { return "potential_data_exposure" }

func (potentialDataExposureRule) Evaluate(d delta.Delta) []api.RiskReason {
	if !publicExposureTriggered(d) {
		return nil
	}

	dataNodeAdded := false
	for _, n := range d.AddedNodes {
		if n.Type == "data" {
			dataNodeAdded = true
			break
		}
	}

	securityRelatedChange := false
	for _, cn := range d.ChangedNodes {
		if cn.Type == "access_control" || cn.Type == "data" {
			securityRelatedChange = true
			break
		}
	}

	if !dataNodeAdded && !securityRelatedChange {
		return nil
	}

	return []api.RiskReason{{
		RuleID:  "potential_data_exposure",
		Message: "Public exposure introduced in presence of data resources or security-related changes. Review potential data exposure risk.",
		Impact:  "data_exposure",
		Weight:  2.0,
	}}
}

// ReviewFocus is a static directive: trace the new public path to any
// data resource. Identical text to the pre-refactor
// interpreter.focusForRule case for "potential_data_exposure".
func (potentialDataExposureRule) ReviewFocus(reason api.RiskReason, d delta.Delta) string {
	return "Trace the new public path to any data resource and confirm it is not reachable from the internet."
}

// publicExposureTriggered mirrors the inner predicate of the
// public_exposure_introduced rule (a changed node whose `public`
// attribute went false -> true). Duplicated here on purpose: the rule
// must be self-contained against the delta so registration order has
// no effect on behavior. If the underlying signal ever changes, both
// places must change together -- a regression test in the risk
// package guards this invariant.
func publicExposureTriggered(d delta.Delta) bool {
	for _, cn := range d.ChangedNodes {
		attr, ok := cn.ChangedAttributes["public"]
		if !ok {
			continue
		}
		before, beforeOK := attr.Before.(bool)
		after, afterOK := attr.After.(bool)
		if !beforeOK || !afterOK {
			continue
		}
		if !before && after {
			return true
		}
	}
	return false
}
