package interpreter

import (
	"strings"
	"testing"

	"architex/delta"
	"architex/risk"
)

// stubInterpreter implements Interpreter with constant outputs, used to verify
// that Render delegates Summary and ReviewFocus to the interface implementation
// (so a future LLM-backed Interpreter can plug in without code changes).
type stubInterpreter struct {
	summary string
	focus   []string
}

func (s stubInterpreter) Summary(_ delta.Delta, _ risk.RiskResult) string {
	return s.summary
}

func (s stubInterpreter) ReviewFocus(_ delta.Delta, _ risk.RiskResult) []string {
	return s.focus
}

func TestRender_UsesProvidedInterpreter(t *testing.T) {
	stub := stubInterpreter{
		summary: "STUB SUMMARY",
		focus:   []string{"STUB BULLET 1", "STUB BULLET 2"},
	}
	d := highRiskDelta()
	r := runRisk(d)
	rep := Render(d, r, stub)

	if rep.Summary != "STUB SUMMARY" {
		t.Errorf("expected stub summary, got %q", rep.Summary)
	}
	if len(rep.ReviewFocus) != 2 || rep.ReviewFocus[0] != "STUB BULLET 1" {
		t.Errorf("expected stub review focus, got %v", rep.ReviewFocus)
	}
	// Diagram must NOT come from the interpreter -- it is part of the trust
	// surface and is always rendered deterministically.
	if !strings.Contains(rep.Diagram, "flowchart LR") {
		t.Errorf("expected deterministic diagram, got %q", rep.Diagram)
	}
}

func TestRender_NilInterpreterFallsBackToDeterministic(t *testing.T) {
	d := highRiskDelta()
	r := runRisk(d)
	rep := Render(d, r, nil)

	if rep.Summary == "" {
		t.Error("expected non-empty summary from default deterministic interpreter")
	}
	if len(rep.ReviewFocus) == 0 {
		t.Error("expected at least one review-focus bullet from default interpreter")
	}
}

func TestRender_ReviewFocusNeverNilInJSON(t *testing.T) {
	stub := stubInterpreter{summary: "x", focus: nil}
	rep := Render(emptyDelta(), runRisk(emptyDelta()), stub)
	if rep.ReviewFocus == nil {
		t.Error("ReviewFocus must be non-nil to serialize as [] not null")
	}
}
