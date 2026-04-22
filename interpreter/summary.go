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

// focusForRule maps a triggered RiskReason to a reviewer-facing
// instruction by dispatching to the rule's own ReviewFocus method via
// the architex/risk registry.
//
// Pre-refactor this was a 100-line switch that duplicated rule IDs and
// hard-coded copy on the interpreter side -- adding a new rule meant
// touching three packages (rule definition + this switch + tests).
// After PR4 of the readability refactor, every migrated rule owns its
// own ReviewFocus method and this function is a one-line dispatcher.
//
// Rules NOT in the registry (today: only the Phase 7 PR5 baseline-
// anomaly rules, which are intentionally registry-less because they
// take *baseline.Baseline) return "" here; the caller skips them. This
// matches the pre-refactor `default: return ""` arm exactly --
// pre-refactor focusForRule had no cases for first_time_*, so it
// already returned "" for them.
func focusForRule(reason risk.RiskReason, d delta.Delta) string {
	if rule := risk.RuleByID(reason.RuleID); rule != nil {
		return rule.ReviewFocus(reason, d)
	}
	return ""
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

// PR3 of the readability refactor removed changedNodeIDs /
// addedNodeIDsByType / removedNodeIDs from this file. They moved to
// architex/risk/rules/internal/rulefmt and are now called by each
// rule's own ReviewFocus method via the registry dispatcher in
// focusForRule above. Keeping the dispatcher single-line here makes
// "where does this bullet text come from?" answerable in one grep.

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
