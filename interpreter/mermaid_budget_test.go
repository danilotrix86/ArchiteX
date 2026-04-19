package interpreter

import (
	"fmt"
	"strings"
	"testing"

	"architex/delta"
	"architex/models"
)

// largeDelta synthesizes a delta with `n` (SG, LB, EC2) triplets. Mirrors the
// shape that scripts/stress-mermaid.ps1 produces against the parser.
func largeDelta(n int) delta.Delta {
	d := delta.Delta{
		AddedNodes:   make([]models.Node, 0, 3*n),
		RemovedNodes: []models.Node{},
		AddedEdges:   make([]models.Edge, 0, 2*n),
		RemovedEdges: []models.Edge{},
		ChangedNodes: []delta.ChangedNode{},
	}
	for i := 1; i <= n; i++ {
		sg := models.Node{ID: fmt.Sprintf("aws_security_group.sg_%d", i), Type: "access_control", ProviderType: "aws_security_group"}
		lb := models.Node{ID: fmt.Sprintf("aws_lb.lb_%d", i), Type: "entry_point", ProviderType: "aws_lb", Attributes: map[string]any{"public": true}}
		ec := models.Node{ID: fmt.Sprintf("aws_instance.ec2_%d", i), Type: "compute", ProviderType: "aws_instance"}
		d.AddedNodes = append(d.AddedNodes, sg, lb, ec)
		d.AddedEdges = append(d.AddedEdges,
			models.Edge{From: lb.ID, To: sg.ID, Type: "attached_to"},
			models.Edge{From: ec.ID, To: sg.ID, Type: "attached_to"},
		)
	}
	d.Summary = delta.DeltaSummary{AddedNodes: 3 * n, AddedEdges: 2 * n}
	return d
}

func TestRenderMermaidBudgeted_SmallDeltaUnchanged(t *testing.T) {
	d := highRiskDelta()
	full := RenderMermaid(d)
	budgeted := RenderMermaidBudgeted(d, MermaidBudget)
	if full != budgeted {
		t.Errorf("small delta should render identically with or without budget\n--- full ---\n%s\n--- budgeted ---\n%s", full, budgeted)
	}
	if strings.Contains(budgeted, "_architex_truncated") {
		t.Errorf("small delta should not contain truncation placeholder")
	}
}

func TestRenderMermaidBudgeted_LargeDeltaTruncates(t *testing.T) {
	d := largeDelta(200)
	out := RenderMermaidBudgeted(d, MermaidBudget)

	if len(out) > MermaidBudget+500 {
		t.Errorf("budgeted output is %d bytes, want <= %d (+500 placeholder slack)", len(out), MermaidBudget)
	}
	if !strings.Contains(out, "_architex_truncated") {
		t.Errorf("large delta must contain truncation placeholder, got:\n%s", out[:min(500, len(out))])
	}
	if !strings.Contains(out, "more node(s)") {
		t.Errorf("placeholder must announce hidden node count, got:\n%s", out[:min(500, len(out))])
	}
}

func TestRenderMermaidBudgeted_DeterministicAcrossRuns(t *testing.T) {
	d := largeDelta(150)
	a := RenderMermaidBudgeted(d, MermaidBudget)
	b := RenderMermaidBudgeted(d, MermaidBudget)
	if a != b {
		t.Fatalf("budgeted render is non-deterministic at the truncation cliff")
	}
}

func TestRenderMermaidBudgeted_PrioritizesHighImpactTypes(t *testing.T) {
	// Tight budget: only ~2-3 nodes will fit. The kept nodes must include
	// at least one entry_point (highest type priority) over any access_control.
	d := largeDelta(50)
	out := RenderMermaidBudgeted(d, 1500)
	if !strings.Contains(out, "entry_point: aws_lb.lb_") {
		t.Errorf("budgeted render at tight budget must keep at least one entry_point node, got:\n%s", out)
	}
}

func TestRenderMermaidBudgeted_BudgetZeroMeansUnlimited(t *testing.T) {
	d := largeDelta(50)
	a := RenderMermaid(d)
	b := RenderMermaidBudgeted(d, 0)
	if a != b {
		t.Errorf("budget 0 should mean 'no cap', expected output identical to RenderMermaid")
	}
}

func TestRenderMermaidBudgeted_KeepsEdgesOnlyWhenBothEndpointsKept(t *testing.T) {
	d := largeDelta(100)
	out := RenderMermaidBudgeted(d, 5000)

	// Every edge line in the kept output must reference IDs that also
	// appear as kept-node lines. Build the kept-node ID set first.
	keptIDs := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if i := strings.Index(line, "["); i > 0 {
			id := line[:i]
			if id != "_architex_truncated" {
				keptIDs[id] = true
			}
		}
	}
	for _, line := range strings.Split(out, "\n") {
		if !strings.Contains(line, "-->") && !strings.Contains(line, "-.->") {
			continue
		}
		if strings.Contains(line, "classDef") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		from := fields[0]
		to := fields[len(fields)-1]
		if !keptIDs[from] || !keptIDs[to] {
			t.Errorf("edge %q references a dropped endpoint (kept set has %d ids)", line, len(keptIDs))
		}
	}
}

