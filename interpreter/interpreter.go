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
	"architex/delta"
	"architex/risk"
)

// SchemaVersion identifies the egress payload schema published in
// docs/egress-schema.json. Bump when the EgressPayload shape changes.
const SchemaVersion = "1.0.0"

// Report is the full Stage 4 output bundle. It is intentionally JSON-friendly
// so callers (CLI, audit writer, future GitHub Action) can persist it as-is.
type Report struct {
	Delta       delta.Delta     `json:"delta"`
	Risk        risk.RiskResult `json:"risk"`
	Diagram     string          `json:"diagram"`
	Summary     string          `json:"summary"`
	ReviewFocus []string        `json:"review_focus"`
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
func Render(d delta.Delta, r risk.RiskResult, interp Interpreter) Report {
	if interp == nil {
		interp = DeterministicInterpreter{}
	}
	focus := interp.ReviewFocus(d, r)
	if focus == nil {
		focus = []string{}
	}
	return Report{
		Delta:       d,
		Risk:        r,
		Diagram:     RenderMermaidBudgeted(d, MermaidBudget),
		Summary:     interp.Summary(d, r),
		ReviewFocus: focus,
	}
}
