package risk

import (
	"fmt"

	"architex/delta"
)

// Rule 1 — Public exposure introduced.
// Triggers when a node's "public" attribute changed from false to true.
func evaluatePublicExposure(d delta.Delta) []RiskReason {
	var reasons []RiskReason
	for _, cn := range d.ChangedNodes {
		attr, ok := cn.ChangedAttributes["public"]
		if !ok {
			continue
		}
		// Defensive type assertion: ChangedAttribute fields are `any`, so a
		// future serialization round-trip could produce non-bool values.
		before, beforeOK := attr.Before.(bool)
		after, afterOK := attr.After.(bool)
		if !beforeOK || !afterOK {
			continue
		}
		if !before && after {
			reasons = append(reasons, RiskReason{
				RuleID:  "public_exposure_introduced",
				Message: fmt.Sprintf("Resource %s became publicly accessible.", cn.ID),
				Impact:  "exposure",
				Weight:  4.0,
			})
		}
	}
	return reasons
}

// Rule 2 — New data resource.
// Triggers for each added node with abstract type "data".
func evaluateNewData(d delta.Delta) []RiskReason {
	var reasons []RiskReason
	for _, n := range d.AddedNodes {
		if n.Type == "data" {
			reasons = append(reasons, RiskReason{
				RuleID:  "new_data_resource",
				Message: fmt.Sprintf("New data resource %s introduced.", n.ID),
				Impact:  "data",
				Weight:  2.5,
			})
		}
	}
	return reasons
}

// Rule 3 — New entry point.
// Triggers for each added node with abstract type "entry_point".
func evaluateNewEntryPoint(d delta.Delta) []RiskReason {
	var reasons []RiskReason
	for _, n := range d.AddedNodes {
		if n.Type == "entry_point" {
			reasons = append(reasons, RiskReason{
				RuleID:  "new_entry_point",
				Message: fmt.Sprintf("New public entry point %s introduced.", n.ID),
				Impact:  "exposure",
				Weight:  3.0,
			})
		}
	}
	return reasons
}

// Rule 4 — Potential data exposure.
// Triggers when public exposure was introduced (Rule 1) AND either:
//   - a data node was added, OR
//   - a changed node is security-related (access_control or data abstract type)
func evaluateDataExposure(d delta.Delta, publicExposureTriggered bool) []RiskReason {
	if !publicExposureTriggered {
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

	return []RiskReason{{
		RuleID:  "potential_data_exposure",
		Message: "Public exposure introduced in presence of data resources or security-related changes. Review potential data exposure risk.",
		Impact:  "data_exposure",
		Weight:  2.0,
	}}
}

// Rule 5 — Resource removal.
// 0.5 per removed node, capped at 2 reasons (total weight 1.0). The cap
// prevents large teardowns from dominating the score.
const removalMaxReasons = 2

func evaluateRemoval(d delta.Delta) []RiskReason {
	var reasons []RiskReason
	for _, n := range d.RemovedNodes {
		if len(reasons) >= removalMaxReasons {
			break
		}
		reasons = append(reasons, RiskReason{
			RuleID:  "resource_removed",
			Message: fmt.Sprintf("Resource %s was removed.", n.ID),
			Impact:  "change",
			Weight:  0.5,
		})
	}
	return reasons
}
