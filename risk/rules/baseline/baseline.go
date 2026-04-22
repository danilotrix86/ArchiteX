// Package baseline houses the three Phase 7 PR5 (v1.2) novelty rules:
// first_time_resource_type, first_time_abstract_type, and
// first_time_edge_pair. They are MIGRATED out of the risk package
// proper, but they do NOT join the architex/risk/api registry.
//
// Why not the registry: every other migrated rule satisfies the
// risk/api.Rule interface (Evaluate(delta.Delta) -> []RiskReason). The
// novelty rules need an additional input -- the *baseline.Baseline
// snapshot -- that the Rule interface does not carry. Pre-refactor the
// orchestrator passed it in via dedicated function calls; we preserve
// that pattern post-refactor by keeping these as plain exported
// functions and having risk.EvaluateWithBaseline call them by name.
//
// A future refactor could either (a) widen the Rule interface to take
// an EvalContext{Delta, Baseline, Config}, or (b) introduce a sibling
// BaselineRule interface. Both are valid; both are out of scope for
// the readability refactor whose contract is "zero behavior change".
//
// Review-focus copy: the pre-refactor interpreter.focusForRule never
// had cases for first_time_*. So these functions intentionally do NOT
// expose a ReviewFocus -- the interpreter will keep returning "" for
// them, which matches the pre-refactor reviewer-facing output exactly.
package baseline

import (
	"fmt"

	"architex/baseline"
	"architex/delta"
	"architex/risk/api"
)

// CapPerRule mirrors rulefmt.DefaultRulePerResourceCap for the novelty
// rules. We keep the constant local to this package so future tuning
// (e.g. dropping novelty caps to 1) does not require touching the
// shared per-resource cap used by every other rule. Same value (2) as
// pre-refactor risk.firstTimeCapPerRule.
const CapPerRule = 2

// EvaluateFirstTimeResourceType is the migrated Phase 7 PR5 Rule 16.
//
// Triggers per added node whose ProviderType has never appeared in the
// baseline. ProviderType is the Terraform-level type (e.g.
// "aws_kms_key"), not the abstract type. This is the most concrete
// signal of the three novelty rules and is what most reviewers will
// read first ("we just adopted KMS in this repo -- new threat model").
//
// De-dup: a single PR that adds three aws_kms_key resources fires the
// rule at most twice (CapPerRule) AND only once per providerType --
// the SAME novel type does not fire on every new instance. We track
// providerTypes locally so the cap interacts cleanly with multi-
// instance modules (PR1 expansion).
func EvaluateFirstTimeResourceType(d delta.Delta, b *baseline.Baseline) []api.RiskReason {
	if b == nil {
		return nil
	}
	var reasons []api.RiskReason
	seen := make(map[string]struct{}, len(d.AddedNodes))
	for _, n := range d.AddedNodes {
		if n.ProviderType == "" {
			continue
		}
		if _, dup := seen[n.ProviderType]; dup {
			continue
		}
		if b.HasProviderType(n.ProviderType) {
			continue
		}
		seen[n.ProviderType] = struct{}{}
		if len(reasons) >= CapPerRule {
			break
		}
		reasons = append(reasons, api.RiskReason{
			RuleID:     "first_time_resource_type",
			Message:    fmt.Sprintf("Resource type %s appears for the first time in this repo (introduced by %s); confirm the team owns the new failure mode and operational surface.", n.ProviderType, n.ID),
			Impact:     "novelty",
			Weight:     1.0,
			ResourceID: n.ID,
		})
	}
	return reasons
}

// EvaluateFirstTimeAbstractType is the migrated Phase 7 PR5 Rule 17.
//
// Triggers when an added node's abstract type (e.g. "entry_point",
// "identity", "storage") has never appeared in the baseline. This is
// the strongest of the three novelty rules: an abstract category is
// broader than a single Terraform type, so "first time we have an
// entry_point" usually marks a real architectural inflection point
// (e.g. "this internal worker repo just gained its first internet-
// exposed surface").
//
// De-dup: at most one reason per abstract type per PR, capped at
// CapPerRule.
func EvaluateFirstTimeAbstractType(d delta.Delta, b *baseline.Baseline) []api.RiskReason {
	if b == nil {
		return nil
	}
	var reasons []api.RiskReason
	seen := make(map[string]struct{}, len(d.AddedNodes))
	for _, n := range d.AddedNodes {
		if n.Type == "" {
			continue
		}
		if _, dup := seen[n.Type]; dup {
			continue
		}
		if b.HasAbstractType(n.Type) {
			continue
		}
		seen[n.Type] = struct{}{}
		if len(reasons) >= CapPerRule {
			break
		}
		reasons = append(reasons, api.RiskReason{
			RuleID:     "first_time_abstract_type",
			Message:    fmt.Sprintf("Abstract type %s appears for the first time in this repo (introduced by %s); a new architectural category usually warrants a review-level decision.", n.Type, n.ID),
			Impact:     "novelty",
			Weight:     1.5,
			ResourceID: n.ID,
		})
	}
	return reasons
}

// EvaluateFirstTimeEdgePair is the migrated Phase 7 PR5 Rule 18.
//
// Triggers when an added edge connects two providerTypes whose pair
// has never appeared in the baseline. Pairs are (sourceProviderType,
// targetProviderType) -- abstract-type pairs are too coarse to be
// useful (every new resource creates an "applies_to" or "deployed_in"
// pair).
//
// We need to look up node provider types from the head graph; rather
// than thread the full graph in, we walk d.AddedNodes plus
// d.ChangedNodes (which both carry ProviderType) and fall back to ""
// when an endpoint is in neither -- in which case the rule does NOT
// fire (we never guess).
//
// Capped at CapPerRule. De-duped by pair so the same novel pair does
// not fire for every parallel-expanded instance.
func EvaluateFirstTimeEdgePair(d delta.Delta, b *baseline.Baseline) []api.RiskReason {
	if b == nil || len(d.AddedEdges) == 0 {
		return nil
	}

	provider := make(map[string]string, len(d.AddedNodes)+len(d.ChangedNodes))
	for _, n := range d.AddedNodes {
		if n.ProviderType != "" {
			provider[n.ID] = n.ProviderType
		}
	}
	for _, n := range d.ChangedNodes {
		if n.ProviderType != "" {
			if _, ok := provider[n.ID]; !ok {
				provider[n.ID] = n.ProviderType
			}
		}
	}

	var reasons []api.RiskReason
	seen := make(map[string]struct{}, len(d.AddedEdges))
	for _, e := range d.AddedEdges {
		from, ok1 := provider[e.From]
		to, ok2 := provider[e.To]
		if !ok1 || !ok2 {
			continue
		}
		key := from + "|" + to
		if _, dup := seen[key]; dup {
			continue
		}
		if b.HasEdgePair(from, to) {
			continue
		}
		seen[key] = struct{}{}
		if len(reasons) >= CapPerRule {
			break
		}
		reasons = append(reasons, api.RiskReason{
			RuleID:     "first_time_edge_pair",
			Message:    fmt.Sprintf("Edge %s -> %s appears for the first time in this repo; confirm the data/control flow it introduces is intentional.", from, to),
			Impact:     "novelty",
			Weight:     0.5,
			ResourceID: e.From,
		})
	}
	return reasons
}
