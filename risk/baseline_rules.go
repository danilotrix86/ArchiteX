package risk

import (
	"fmt"

	"architex/baseline"
	"architex/delta"
)

// ---------------------------------------------------------------------------
// Phase 7 PR5 (v1.2) — Baseline anomaly rules.
//
// These three rules answer "is this kind of thing the FIRST of its kind to
// land in this repo?". They consume a *baseline.Baseline snapshot
// committed to the repo (default `.architex/baseline.json`). A nil
// baseline disables the entire group; this is the bit-identical-to-v1.1
// fallback documented in the package comment of `baseline`.
//
// Weights are intentionally low (1.0 - 1.5) -- novelty is informational.
// A first-time entry_point is also caught by `new_entry_point` (3.0); the
// baseline rule layers on top, so the same PR scores 4.5 instead of 3.0,
// nudging reviewers to look harder at "we have never had a CloudFront
// before, are we sure we want one?". Per-rule cap matches Phase 6 (2
// reasons max) so a sweeping infra-rewrite cannot saturate the score.
// ---------------------------------------------------------------------------

// firstTimeCapPerRule mirrors phase6CapPerRule for the baseline rules. We
// keep them on a separate symbol so future tuning (e.g. dropping novelty
// caps to 1) does not require touching Phase 6 constants.
const firstTimeCapPerRule = 2

// Rule 16 — first_time_resource_type.
//
// Triggers per added node whose ProviderType has never appeared in the
// baseline. ProviderType is the Terraform-level type (e.g. "aws_kms_key"),
// not the abstract type. This is the most concrete signal of the three
// novelty rules and is what most reviewers will read first ("we just
// adopted KMS in this repo -- new threat model").
//
// De-dup: a single PR that adds three aws_kms_key resources fires the
// rule at most twice (`firstTimeCapPerRule`) AND only once per
// providerType -- the SAME novel type does not fire on every new
// instance. We track providerTypes locally so the cap interacts cleanly
// with multi-instance modules (PR1 expansion).
func evaluateFirstTimeResourceType(d delta.Delta, b *baseline.Baseline) []RiskReason {
	if b == nil {
		return nil
	}
	var reasons []RiskReason
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
		if len(reasons) >= firstTimeCapPerRule {
			break
		}
		reasons = append(reasons, RiskReason{
			RuleID:     "first_time_resource_type",
			Message:    fmt.Sprintf("Resource type %s appears for the first time in this repo (introduced by %s); confirm the team owns the new failure mode and operational surface.", n.ProviderType, n.ID),
			Impact:     "novelty",
			Weight:     1.0,
			ResourceID: n.ID,
		})
	}
	return reasons
}

// Rule 17 — first_time_abstract_type.
//
// Triggers when an added node's abstract type (e.g. "entry_point",
// "identity", "storage") has never appeared in the baseline. This is the
// strongest of the three novelty rules: an abstract category is broader
// than a single Terraform type, so "first time we have an entry_point"
// usually marks a real architectural inflection point (e.g. "this
// internal worker repo just gained its first internet-exposed surface").
//
// De-dup: at most one reason per abstract type per PR, capped at 2.
func evaluateFirstTimeAbstractType(d delta.Delta, b *baseline.Baseline) []RiskReason {
	if b == nil {
		return nil
	}
	var reasons []RiskReason
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
		if len(reasons) >= firstTimeCapPerRule {
			break
		}
		reasons = append(reasons, RiskReason{
			RuleID:     "first_time_abstract_type",
			Message:    fmt.Sprintf("Abstract type %s appears for the first time in this repo (introduced by %s); a new architectural category usually warrants a review-level decision.", n.Type, n.ID),
			Impact:     "novelty",
			Weight:     1.5,
			ResourceID: n.ID,
		})
	}
	return reasons
}

// Rule 18 — first_time_edge_pair.
//
// Triggers when an added edge connects two providerTypes whose pair has
// never appeared in the baseline. Pairs are (sourceProviderType,
// targetProviderType) -- abstract-type pairs are too coarse to be useful
// (every new resource creates an "applies_to" or "deployed_in" pair).
//
// We need to look up node provider types from the head graph; rather than
// thread the full graph in, we walk d.AddedNodes plus d.ChangedNodes
// (which both carry ProviderType) and fall back to "" when an endpoint
// is in neither -- in which case the rule does NOT fire (we never guess).
//
// Capped at 2 reasons per evaluation. De-duped by pair so the same novel
// pair does not fire for every parallel-expanded instance.
func evaluateFirstTimeEdgePair(d delta.Delta, b *baseline.Baseline) []RiskReason {
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

	var reasons []RiskReason
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
		if len(reasons) >= firstTimeCapPerRule {
			break
		}
		reasons = append(reasons, RiskReason{
			RuleID:     "first_time_edge_pair",
			Message:    fmt.Sprintf("Edge %s -> %s appears for the first time in this repo; confirm the data/control flow it introduces is intentional.", from, to),
			Impact:     "novelty",
			Weight:     0.5,
			ResourceID: e.From,
		})
	}
	return reasons
}
