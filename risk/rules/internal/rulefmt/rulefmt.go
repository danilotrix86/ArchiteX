// Package rulefmt is a tiny shared-helpers package for the rule
// subpackages under architex/risk/rules/. It owns ONLY the formatting
// primitives used by Rule.ReviewFocus implementations -- delta queries
// (added-by-type, changed-by-attr, removed) and human-readable list
// joining.
//
// Why a separate package (vs inlining in each rule):
//
//   - The same handful of helpers are needed by ~5 different rules across
//     3 different domain packages (exposure, data, lifecycle). Inlining
//     would mean five near-identical copies.
//   - The helpers are small, pure, and have no risk-domain semantics --
//     they belong in a util package, not in any one rule's file.
//
// Why under risk/rules/internal/ (vs e.g. internal/text):
//
//   - Go's "internal" convention scopes consumers to architex/risk/rules/...
//     This is the precise blast radius we want: rule subpackages, and
//     nothing else. The interpreter has its OWN equivalents (and keeps
//     them) because PR3-PR4 of the readability refactor leaves the
//     interpreter free of any internal-rules dependency.
package rulefmt

import (
	"sort"
	"strings"

	"architex/delta"
)

// JoinHumanList joins items with commas and an Oxford-style "and" before
// the last item. Returns "" for nil/empty input. Mirrors the
// interpreter.joinHumanList helper byte-for-byte so review-focus strings
// stay identical when a rule's ReviewFocus method takes over from the
// pre-refactor interpreter.focusForRule switch.
func JoinHumanList(items []string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	case 2:
		return items[0] + " and " + items[1]
	default:
		return strings.Join(items[:len(items)-1], ", ") + ", and " + items[len(items)-1]
	}
}

// ChangedNodeIDsByAttr returns the IDs of every changed node whose
// ChangedAttributes map carries the given key, sorted ascending. Mirrors
// interpreter.changedNodeIDs verbatim. Used by the public_exposure_introduced
// review-focus aggregator so the bullet lists every newly-public resource
// rather than emitting one bullet per resource.
func ChangedNodeIDsByAttr(d delta.Delta, attrKey string) []string {
	var out []string
	for _, cn := range d.ChangedNodes {
		if _, ok := cn.ChangedAttributes[attrKey]; ok {
			out = append(out, cn.ID)
		}
	}
	sort.Strings(out)
	return out
}

// AddedNodeIDsByType returns the IDs of every added node whose abstract
// Type matches, sorted ascending. Mirrors interpreter.addedNodeIDsByType
// verbatim. Used by new_data_resource and new_entry_point review-focus
// aggregators.
func AddedNodeIDsByType(d delta.Delta, abstractType string) []string {
	var out []string
	for _, n := range d.AddedNodes {
		if n.Type == abstractType {
			out = append(out, n.ID)
		}
	}
	sort.Strings(out)
	return out
}

// RemovedNodeIDs returns the IDs of every removed node, sorted ascending.
// Mirrors interpreter.removedNodeIDs verbatim. Used by the
// resource_removed review-focus aggregator.
func RemovedNodeIDs(d delta.Delta) []string {
	out := make([]string, 0, len(d.RemovedNodes))
	for _, n := range d.RemovedNodes {
		out = append(out, n.ID)
	}
	sort.Strings(out)
	return out
}

// DefaultRulePerResourceCap bounds the number of findings ANY one
// per-resource rule may emit in a single evaluation. The cap exists so a
// sweeping refactor (e.g. adding ten public CloudFront distributions in
// one PR) cannot single-handedly saturate the 10.0 score cap. A reviewer
// who sees 2 instances of the same finding already understands the
// pattern; the rest add no information.
//
// Pre-refactor this constant lived in risk/rules.go as
// `phase6CapPerRule`. Renamed during PR3 of the readability refactor to
// describe the SEMANTICS ("how many findings per rule per evaluation")
// rather than the RELEASE ("phase 6 cap"). Same value (2) carries over.
const DefaultRulePerResourceCap = 2

// IsConditional returns true when a node's Attributes carry the
// `conditional = true` marker the parser writes for resources whose own
// existence depends on an unresolvable expression (e.g.
// `count = var.create ? 1 : 0`). Risk rules MUST treat conditional
// nodes as non-existent for scoring purposes -- the engine never
// invents findings on resources whose own existence is conditional.
//
// Mirrors risk.isConditionalNode (the pre-refactor private helper) byte-
// for-byte. The new name drops the redundant "Node" suffix; all
// migrated rules use this identifier.
func IsConditional(attrs map[string]any) bool {
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
