// Package interpreter is Stage 4 of the ArchiteX pipeline. It turns a Delta
// and a RiskResult into human-facing artifacts: a Mermaid delta diagram, a
// plain-English summary, review-focus bullets, a Markdown PR comment, and
// a self-contained HTML report.
//
// The package is fully deterministic and template-based. There is no
// inference, no model call, and no network dependency anywhere in the
// rendering pipeline -- the same Delta + RiskResult produces byte-identical
// output across runs and across machines. The Interpreter interface exists
// purely as a clean seam for alternative deterministic renderers (custom
// summary templates, alternative output formats, locale variants); any
// implementation must remain deterministic-equivalent.
package interpreter

import (
	"sort"
	"strings"

	"architex/delta"
	"architex/models"
	"architex/risk"
)

// SchemaVersion identifies the egress payload schema published in
// docs/egress-schema.json. Bump when the EgressPayload shape changes.
const SchemaVersion = "1.0.0"

// Report is the full Stage 4 output bundle. It is intentionally JSON-friendly
// so callers (CLI, audit writer, future GitHub Action) can persist it as-is.
//
// Providers and ResourceCount are populated by RenderWithGraph (v1.4+) from
// the full head graph so the cosmetic provider banner can describe the
// whole repository, not just the delta. They are intentionally omitted when
// Render is used without a graph -- callers that don't supply a head graph
// fall back to the delta-derived banner (delta-only providers, delta-only
// node count) so v1.3-and-earlier integrations keep working unchanged.
type Report struct {
	Delta         delta.Delta     `json:"delta"`
	Risk          risk.RiskResult `json:"risk"`
	Diagram       string          `json:"diagram"`
	Summary       string          `json:"summary"`
	ReviewFocus   []string        `json:"review_focus"`
	Providers     []string        `json:"providers,omitempty"`
	ResourceCount int             `json:"resource_count,omitempty"`
}

// Interpreter is the seam where an alternative deterministic renderer can
// be slotted in (custom summary templates, locale variants, alternative
// output styles). Implementations must be deterministic-equivalent --
// same input produces the same shape of output, every time -- so that
// downstream artifacts remain reproducible. Production trust-critical
// decisions never depend on this interface; it shapes presentation only.
type Interpreter interface {
	Summary(d delta.Delta, r risk.RiskResult) string
	ReviewFocus(d delta.Delta, r risk.RiskResult) []string
}

// Render builds a full Report from a Delta and a RiskResult.
//
// If interp is nil the DeterministicInterpreter is used. The Mermaid diagram
// is always produced deterministically -- it does NOT go through the
// Interpreter interface because the diagram is part of the trust surface.
//
// For v1.4+ multi-provider repos prefer RenderWithGraph -- it populates the
// repository-wide provider banner. Render is preserved as a thin wrapper so
// pre-v1.4 callers and tests keep working bit-identically.
func Render(d delta.Delta, r risk.RiskResult, interp Interpreter) Report {
	return RenderWithGraph(d, r, models.Graph{}, interp)
}

// RenderWithGraph is the v1.4 entry point. It accepts the head graph so the
// cosmetic provider banner reflects the whole repository -- not just the
// nodes that happened to change in this PR. Passing a zero-value head
// (e.g. models.Graph{}) yields the same Report shape as Render and keeps
// the delta-derived banner fallback intact.
//
// Like Render, the diagram is produced deterministically and the Interpreter
// shapes only the human-facing summary/focus text.
func RenderWithGraph(d delta.Delta, r risk.RiskResult, head models.Graph, interp Interpreter) Report {
	if interp == nil {
		interp = DeterministicInterpreter{}
	}
	focus := interp.ReviewFocus(d, r)
	if focus == nil {
		focus = []string{}
	}
	providers, count := providersFromGraph(head)
	return Report{
		Delta:         d,
		Risk:          r,
		Diagram:       RenderMermaidBudgeted(d, MermaidBudget),
		Summary:       interp.Summary(d, r),
		ReviewFocus:   focus,
		Providers:     providers,
		ResourceCount: count,
	}
}

// providersFromGraph extracts the deterministic, sorted, deduplicated list
// of provider prefixes (the substring before the first underscore in
// Node.ProviderType, e.g. "aws", "azurerm") for every node in g, plus the
// total node count. Returns (nil, 0) when g has no nodes so the Report
// keeps the omitempty contract for graph-less callers.
func providersFromGraph(g models.Graph) ([]string, int) {
	if len(g.Nodes) == 0 {
		return nil, 0
	}
	seen := make(map[string]bool, 4)
	for _, n := range g.Nodes {
		pt := n.ProviderType
		if pt == "" {
			continue
		}
		i := strings.IndexByte(pt, '_')
		var prefix string
		if i <= 0 {
			prefix = pt
		} else {
			prefix = pt[:i]
		}
		seen[prefix] = true
	}
	if len(seen) == 0 {
		return nil, len(g.Nodes)
	}
	out := make([]string, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	sort.Strings(out)
	return out, len(g.Nodes)
}
