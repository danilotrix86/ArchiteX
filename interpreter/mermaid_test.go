package interpreter

import (
	"strings"
	"testing"

	"architex/models"
)

func TestRenderMermaid_HighRiskHasAddedChangedAndContext(t *testing.T) {
	out := RenderMermaid(highRiskDelta())

	mustContain := []string{
		"flowchart LR",
		"classDef added",
		"classDef removed",
		"classDef changed",
		"classDef context",
		`aws_lb_web["+ entry_point: aws_lb.web"]:::added`,
		`aws_security_group_web["~ access_control: aws_security_group.web"]:::changed`,
		`aws_subnet_public["network: aws_subnet.public"]:::context`,
		"aws_lb_web -->|attached_to| aws_security_group_web",
		"aws_lb_web -->|deployed_in| aws_subnet_public",
	}
	for _, want := range mustContain {
		if !strings.Contains(out, want) {
			t.Errorf("expected diagram to contain %q\n--- diagram ---\n%s", want, out)
		}
	}

	// Context node should NOT carry an added/removed/changed marker.
	if strings.Contains(out, "+ network: aws_subnet.public") {
		t.Errorf("context node aws_subnet.public should not have an added marker")
	}
}

func TestRenderMermaid_DeterministicAcrossRuns(t *testing.T) {
	d := highRiskDelta()
	a := RenderMermaid(d)
	b := RenderMermaid(d)
	if a != b {
		t.Fatalf("RenderMermaid is non-deterministic:\n--- a ---\n%s\n--- b ---\n%s", a, b)
	}
}

func TestRenderMermaid_EmptyDelta(t *testing.T) {
	out := RenderMermaid(emptyDelta())
	if !strings.Contains(out, "flowchart LR") {
		t.Errorf("empty delta should still produce a flowchart header, got:\n%s", out)
	}
	if !strings.Contains(out, "no architectural changes") {
		t.Errorf("empty delta should announce no changes, got:\n%s", out)
	}
}

func TestRenderMermaid_RemovedEdgeUsesDashedArrow(t *testing.T) {
	d := emptyDelta()
	d.RemovedEdges = append(d.RemovedEdges, models.Edge{
		From: "aws_instance.web",
		To:   "aws_security_group.web",
		Type: "attached_to",
	})
	d.Summary.RemovedEdges = 1

	out := RenderMermaid(d)
	if !strings.Contains(out, "-.->") {
		t.Errorf("removed edge should render with dashed arrow -.->, got:\n%s", out)
	}
	// Removed-edge endpoints become context nodes (no marker).
	if !strings.Contains(out, `aws_instance_web["compute: aws_instance.web"]:::context`) {
		t.Errorf("expected unchanged endpoint to render as context, got:\n%s", out)
	}
}

func TestSanitizeID_ReplacesNonIdentifierChars(t *testing.T) {
	cases := map[string]string{
		"aws_security_group.web":    "aws_security_group_web",
		"aws_lb.web-prod":           "aws_lb_web_prod",
		"module.foo.aws_instance.x": "module_foo_aws_instance_x",
		"already_safe_id":           "already_safe_id",
	}
	for in, want := range cases {
		if got := sanitizeID(in); got != want {
			t.Errorf("sanitizeID(%q) = %q, want %q", in, got, want)
		}
	}
}
