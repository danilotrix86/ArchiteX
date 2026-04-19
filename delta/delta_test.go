package delta

import (
	"testing"

	"architex/models"
)

func TestCompare_AddedNode(t *testing.T) {
	base := models.Graph{
		Nodes: []models.Node{
			{ID: "aws_vpc.main", Type: "network", ProviderType: "aws_vpc", Attributes: map[string]any{"public": false}},
		},
	}
	head := models.Graph{
		Nodes: []models.Node{
			{ID: "aws_vpc.main", Type: "network", ProviderType: "aws_vpc", Attributes: map[string]any{"public": false}},
			{ID: "aws_db_instance.main", Type: "data", ProviderType: "aws_db_instance", Attributes: map[string]any{"public": false}},
		},
	}

	d := Compare(base, head)

	if d.Summary.AddedNodes != 1 {
		t.Fatalf("expected 1 added node, got %d", d.Summary.AddedNodes)
	}
	if d.AddedNodes[0].ID != "aws_db_instance.main" {
		t.Fatalf("expected added node aws_db_instance.main, got %s", d.AddedNodes[0].ID)
	}
	if d.Summary.RemovedNodes != 0 {
		t.Fatalf("expected 0 removed nodes, got %d", d.Summary.RemovedNodes)
	}
	if d.Summary.ChangedNodes != 0 {
		t.Fatalf("expected 0 changed nodes, got %d", d.Summary.ChangedNodes)
	}
}

func TestCompare_RemovedNode(t *testing.T) {
	base := models.Graph{
		Nodes: []models.Node{
			{ID: "aws_vpc.main", Type: "network", ProviderType: "aws_vpc", Attributes: map[string]any{"public": false}},
			{ID: "aws_instance.web", Type: "compute", ProviderType: "aws_instance", Attributes: map[string]any{"public": true}},
		},
	}
	head := models.Graph{
		Nodes: []models.Node{
			{ID: "aws_vpc.main", Type: "network", ProviderType: "aws_vpc", Attributes: map[string]any{"public": false}},
		},
	}

	d := Compare(base, head)

	if d.Summary.RemovedNodes != 1 {
		t.Fatalf("expected 1 removed node, got %d", d.Summary.RemovedNodes)
	}
	if d.RemovedNodes[0].ID != "aws_instance.web" {
		t.Fatalf("expected removed node aws_instance.web, got %s", d.RemovedNodes[0].ID)
	}
	if d.Summary.AddedNodes != 0 {
		t.Fatalf("expected 0 added nodes, got %d", d.Summary.AddedNodes)
	}
}

func TestCompare_AddedEdge(t *testing.T) {
	nodes := []models.Node{
		{ID: "aws_instance.web", Type: "compute", ProviderType: "aws_instance", Attributes: map[string]any{"public": true}},
		{ID: "aws_security_group.web", Type: "access_control", ProviderType: "aws_security_group", Attributes: map[string]any{"public": false}},
	}
	base := models.Graph{Nodes: nodes}
	head := models.Graph{
		Nodes: nodes,
		Edges: []models.Edge{
			{From: "aws_instance.web", To: "aws_security_group.web", Type: "attached_to"},
		},
	}

	d := Compare(base, head)

	if d.Summary.AddedEdges != 1 {
		t.Fatalf("expected 1 added edge, got %d", d.Summary.AddedEdges)
	}
	if d.AddedEdges[0].From != "aws_instance.web" || d.AddedEdges[0].To != "aws_security_group.web" {
		t.Fatalf("unexpected added edge: %+v", d.AddedEdges[0])
	}
	if d.Summary.RemovedEdges != 0 {
		t.Fatalf("expected 0 removed edges, got %d", d.Summary.RemovedEdges)
	}
}

func TestCompare_RemovedEdge(t *testing.T) {
	nodes := []models.Node{
		{ID: "aws_instance.web", Type: "compute", ProviderType: "aws_instance", Attributes: map[string]any{"public": true}},
		{ID: "aws_subnet.public", Type: "network", ProviderType: "aws_subnet", Attributes: map[string]any{"public": false}},
	}
	base := models.Graph{
		Nodes: nodes,
		Edges: []models.Edge{
			{From: "aws_instance.web", To: "aws_subnet.public", Type: "deployed_in"},
		},
	}
	head := models.Graph{Nodes: nodes}

	d := Compare(base, head)

	if d.Summary.RemovedEdges != 1 {
		t.Fatalf("expected 1 removed edge, got %d", d.Summary.RemovedEdges)
	}
	if d.RemovedEdges[0].From != "aws_instance.web" || d.RemovedEdges[0].To != "aws_subnet.public" {
		t.Fatalf("unexpected removed edge: %+v", d.RemovedEdges[0])
	}
	if d.Summary.AddedEdges != 0 {
		t.Fatalf("expected 0 added edges, got %d", d.Summary.AddedEdges)
	}
}

func TestCompare_ChangedAttribute(t *testing.T) {
	base := models.Graph{
		Nodes: []models.Node{
			{ID: "aws_security_group.web", Type: "access_control", ProviderType: "aws_security_group", Attributes: map[string]any{"public": false}},
		},
	}
	head := models.Graph{
		Nodes: []models.Node{
			{ID: "aws_security_group.web", Type: "access_control", ProviderType: "aws_security_group", Attributes: map[string]any{"public": true}},
		},
	}

	d := Compare(base, head)

	if d.Summary.ChangedNodes != 1 {
		t.Fatalf("expected 1 changed node, got %d", d.Summary.ChangedNodes)
	}
	cn := d.ChangedNodes[0]
	if cn.ID != "aws_security_group.web" {
		t.Fatalf("expected changed node aws_security_group.web, got %s", cn.ID)
	}
	attr, ok := cn.ChangedAttributes["public"]
	if !ok {
		t.Fatal("expected changed_attributes to contain 'public'")
	}
	if attr.Before != false {
		t.Fatalf("expected before=false, got %v", attr.Before)
	}
	if attr.After != true {
		t.Fatalf("expected after=true, got %v", attr.After)
	}
	if cn.Type != "access_control" {
		t.Fatalf("expected ChangedNode.Type=access_control, got %q", cn.Type)
	}
	if cn.ProviderType != "aws_security_group" {
		t.Fatalf("expected ChangedNode.ProviderType=aws_security_group, got %q", cn.ProviderType)
	}
	if d.Summary.AddedNodes != 0 || d.Summary.RemovedNodes != 0 {
		t.Fatal("expected no added or removed nodes")
	}
}

func TestCompare_UnchangedGraph(t *testing.T) {
	g := models.Graph{
		Nodes: []models.Node{
			{ID: "aws_vpc.main", Type: "network", ProviderType: "aws_vpc", Attributes: map[string]any{"public": false}},
			{ID: "aws_instance.web", Type: "compute", ProviderType: "aws_instance", Attributes: map[string]any{"public": true}},
		},
		Edges: []models.Edge{
			{From: "aws_instance.web", To: "aws_vpc.main", Type: "references"},
		},
	}

	d := Compare(g, g)

	if d.Summary.AddedNodes != 0 || d.Summary.RemovedNodes != 0 ||
		d.Summary.AddedEdges != 0 || d.Summary.RemovedEdges != 0 ||
		d.Summary.ChangedNodes != 0 {
		t.Fatalf("expected all-zero summary for unchanged graph, got %+v", d.Summary)
	}
	if len(d.AddedNodes) != 0 || len(d.RemovedNodes) != 0 ||
		len(d.AddedEdges) != 0 || len(d.RemovedEdges) != 0 ||
		len(d.ChangedNodes) != 0 {
		t.Fatal("expected empty delta slices for unchanged graph")
	}
}

func TestCompare_StableOrdering(t *testing.T) {
	base := models.Graph{}
	head := models.Graph{
		Nodes: []models.Node{
			{ID: "aws_lb.web", Type: "entry_point", ProviderType: "aws_lb", Attributes: map[string]any{"public": true}},
			{ID: "aws_db_instance.main", Type: "data", ProviderType: "aws_db_instance", Attributes: map[string]any{"public": false}},
			{ID: "aws_instance.web", Type: "compute", ProviderType: "aws_instance", Attributes: map[string]any{"public": true}},
		},
		Edges: []models.Edge{
			{From: "aws_lb.web", To: "aws_security_group.web", Type: "attached_to"},
			{From: "aws_db_instance.main", To: "aws_security_group.db", Type: "attached_to"},
			{From: "aws_instance.web", To: "aws_subnet.public", Type: "deployed_in"},
			{From: "aws_instance.web", To: "aws_security_group.web", Type: "attached_to"},
		},
	}

	d := Compare(base, head)

	// Nodes: sorted by ID alphabetically.
	expectedNodeOrder := []string{"aws_db_instance.main", "aws_instance.web", "aws_lb.web"}
	for i, want := range expectedNodeOrder {
		if d.AddedNodes[i].ID != want {
			t.Fatalf("added node [%d]: expected %s, got %s", i, want, d.AddedNodes[i].ID)
		}
	}

	// Edges: sorted by From, then To, then Type.
	expectedEdgeOrder := []struct{ from, to string }{
		{"aws_db_instance.main", "aws_security_group.db"},
		{"aws_instance.web", "aws_security_group.web"},
		{"aws_instance.web", "aws_subnet.public"},
		{"aws_lb.web", "aws_security_group.web"},
	}
	for i, want := range expectedEdgeOrder {
		if d.AddedEdges[i].From != want.from || d.AddedEdges[i].To != want.to {
			t.Fatalf("added edge [%d]: expected %s->%s, got %s->%s",
				i, want.from, want.to, d.AddedEdges[i].From, d.AddedEdges[i].To)
		}
	}
}

func TestHumanSummary(t *testing.T) {
	d := Delta{
		Summary: DeltaSummary{
			AddedNodes:   1,
			RemovedNodes: 0,
			AddedEdges:   2,
			RemovedEdges: 0,
			ChangedNodes: 1,
		},
	}
	got := HumanSummary(d)
	want := "1 node added, 2 edges added, 1 node changed"
	if got != want {
		t.Fatalf("HumanSummary:\n  got:  %q\n  want: %q", got, want)
	}
}

func TestHumanSummary_NoChanges(t *testing.T) {
	d := Delta{}
	got := HumanSummary(d)
	if got != "no changes" {
		t.Fatalf("expected 'no changes', got %q", got)
	}
}
