package interpreter

import (
	"fmt"
	"sort"
	"strings"

	"architex/delta"
	"architex/risk"
)

// DeterministicInterpreter generates summary text and review-focus bullets
// from RiskReason and Delta data using fixed templates. No external calls,
// no probabilistic logic. Same input always produces same output.
type DeterministicInterpreter struct{}

// Summary returns a one-paragraph plain-English description of the change.
// Shape mirrors master.md §10.1 example output. When the delta is empty the
// returned summary explicitly states there were no architectural changes.
func (DeterministicInterpreter) Summary(d delta.Delta, r risk.RiskResult) string {
	if isEmptyDelta(d) {
		return "No architectural changes were detected in this pull request."
	}

	var clauses []string

	if added := classifyAdded(d); len(added) > 0 {
		clauses = append(clauses, fmt.Sprintf("Added %s.", joinHumanList(added)))
	}
	if removed := classifyRemoved(d); len(removed) > 0 {
		clauses = append(clauses, fmt.Sprintf("Removed %s.", joinHumanList(removed)))
	}
	if changed := classifyChanged(d); len(changed) > 0 {
		clauses = append(clauses, fmt.Sprintf("Modified %s.", joinHumanList(changed)))
	}
	if d.Summary.AddedEdges > 0 || d.Summary.RemovedEdges > 0 {
		clauses = append(clauses, fmt.Sprintf("%s.", edgeClause(d)))
	}

	if exposure := exposureClause(r); exposure != "" {
		clauses = append(clauses, exposure)
	}

	if len(clauses) == 0 {
		return "Architectural changes were detected, but none matched a known risk pattern."
	}
	return strings.Join(clauses, " ")
}

// ReviewFocus returns ordered, deduplicated bullet points telling the
// reviewer where to look first. Output is sorted by rule severity (highest
// weight first) so the most important guidance appears at the top.
func (DeterministicInterpreter) ReviewFocus(d delta.Delta, r risk.RiskResult) []string {
	if len(r.Reasons) == 0 && isEmptyDelta(d) {
		return []string{"No review focus required: no architectural changes detected."}
	}

	seen := make(map[string]bool)
	var out []string

	for _, reason := range r.Reasons {
		bullet := focusForRule(reason, d)
		if bullet == "" || seen[bullet] {
			continue
		}
		seen[bullet] = true
		out = append(out, bullet)
	}

	if len(out) == 0 {
		out = append(out, "No risk rules triggered. Review the change for intent and naming consistency.")
	}
	return out
}

// focusForRule maps a triggered RiskReason to a reviewer-facing instruction.
// Keying on RuleID (a known constant) keeps this purely deterministic and
// avoids parsing free-form Message text.
func focusForRule(reason risk.RiskReason, d delta.Delta) string {
	switch reason.RuleID {
	case "public_exposure_introduced":
		ids := changedNodeIDs(d, "public")
		return fmt.Sprintf(
			"Confirm that public exposure is intended on %s and that ingress is restricted to required ports and sources.",
			joinHumanList(ids),
		)
	case "new_data_resource":
		ids := addedNodeIDsByType(d, "data")
		return fmt.Sprintf(
			"Verify encryption at rest, backups, and access controls on the new data resource(s): %s.",
			joinHumanList(ids),
		)
	case "new_entry_point":
		ids := addedNodeIDsByType(d, "entry_point")
		return fmt.Sprintf(
			"Review the new entry point(s) %s for TLS, authentication, and exposure scope.",
			joinHumanList(ids),
		)
	case "potential_data_exposure":
		return "Trace the new public path to any data resource and confirm it is not reachable from the internet."
	case "resource_removed":
		ids := removedNodeIDs(d)
		return fmt.Sprintf(
			"Confirm the removal of %s is intended; check for orphaned dependencies and audit log retention.",
			joinHumanList(ids),
		)
	default:
		return ""
	}
}

// Helpers ---------------------------------------------------------------------

func isEmptyDelta(d delta.Delta) bool {
	return d.Summary.AddedNodes == 0 &&
		d.Summary.RemovedNodes == 0 &&
		d.Summary.AddedEdges == 0 &&
		d.Summary.RemovedEdges == 0 &&
		d.Summary.ChangedNodes == 0
}

func classifyAdded(d delta.Delta) []string {
	counts := make(map[string]int)
	for _, n := range d.AddedNodes {
		counts[n.Type]++
	}
	return formatTypeCounts(counts)
}

func classifyRemoved(d delta.Delta) []string {
	counts := make(map[string]int)
	for _, n := range d.RemovedNodes {
		counts[n.Type]++
	}
	return formatTypeCounts(counts)
}

func classifyChanged(d delta.Delta) []string {
	counts := make(map[string]int)
	for _, cn := range d.ChangedNodes {
		counts[cn.Type]++
	}
	return formatTypeCounts(counts)
}

func formatTypeCounts(counts map[string]int) []string {
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		n := counts[k]
		if n == 1 {
			out = append(out, fmt.Sprintf("1 %s resource", humanType(k)))
		} else {
			out = append(out, fmt.Sprintf("%d %s resources", n, humanType(k)))
		}
	}
	return out
}

func edgeClause(d delta.Delta) string {
	var parts []string
	if n := d.Summary.AddedEdges; n > 0 {
		parts = append(parts, fmt.Sprintf("%d new dependency edge%s", n, pluralS(n)))
	}
	if n := d.Summary.RemovedEdges; n > 0 {
		parts = append(parts, fmt.Sprintf("%d removed dependency edge%s", n, pluralS(n)))
	}
	return "Connectivity changed: " + strings.Join(parts, " and ")
}

func exposureClause(r risk.RiskResult) string {
	for _, reason := range r.Reasons {
		if reason.RuleID == "public_exposure_introduced" {
			return "A previously private resource is now publicly accessible, increasing the blast radius of this change."
		}
	}
	return ""
}

func changedNodeIDs(d delta.Delta, attrKey string) []string {
	var out []string
	for _, cn := range d.ChangedNodes {
		if _, ok := cn.ChangedAttributes[attrKey]; ok {
			out = append(out, cn.ID)
		}
	}
	sort.Strings(out)
	return out
}

func addedNodeIDsByType(d delta.Delta, abstractType string) []string {
	var out []string
	for _, n := range d.AddedNodes {
		if n.Type == abstractType {
			out = append(out, n.ID)
		}
	}
	sort.Strings(out)
	return out
}

func removedNodeIDs(d delta.Delta) []string {
	out := make([]string, 0, len(d.RemovedNodes))
	for _, n := range d.RemovedNodes {
		out = append(out, n.ID)
	}
	sort.Strings(out)
	return out
}

// humanType converts an abstract type ("entry_point") into a noun phrase
// suitable for inline prose ("entry-point").
func humanType(t string) string {
	return strings.ReplaceAll(t, "_", "-")
}

func pluralS(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// joinHumanList joins items with commas, using "and" before the last item.
// Returns an empty string for nil/empty input. Single item returned as-is.
func joinHumanList(items []string) string {
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
