package graph

import (
	"testing"

	"architex/models"
)

func TestBuild_NodesAndEdges(t *testing.T) {
	resources := []models.RawResource{
		{
			Type: "aws_vpc", Name: "main", ID: "aws_vpc.main",
			Attributes: map[string]any{"cidr_block": "10.0.0.0/16"},
		},
		{
			Type: "aws_subnet", Name: "web", ID: "aws_subnet.web",
			Attributes: map[string]any{"cidr_block": "10.0.1.0/24"},
			References: []models.Reference{
				{SourceAttr: "vpc_id", TargetID: "aws_vpc.main"},
			},
		},
		{
			Type: "aws_security_group", Name: "web", ID: "aws_security_group.web",
			Attributes: map[string]any{"cidr_blocks": []any{"0.0.0.0/0"}},
			References: []models.Reference{
				{SourceAttr: "vpc_id", TargetID: "aws_vpc.main"},
			},
		},
		{
			Type: "aws_instance", Name: "web", ID: "aws_instance.web",
			Attributes: map[string]any{"associate_public_ip_address": true},
			References: []models.Reference{
				{SourceAttr: "subnet_id", TargetID: "aws_subnet.web"},
				{SourceAttr: "vpc_security_group_ids", TargetID: "aws_security_group.web"},
			},
		},
		{
			Type: "aws_lb", Name: "web", ID: "aws_lb.web",
			Attributes: map[string]any{},
			References: []models.Reference{
				{SourceAttr: "security_groups", TargetID: "aws_security_group.web"},
				{SourceAttr: "subnets", TargetID: "aws_subnet.web"},
			},
		},
		{
			Type: "aws_db_instance", Name: "main", ID: "aws_db_instance.main",
			Attributes: map[string]any{},
			References: []models.Reference{
				{SourceAttr: "vpc_security_group_ids", TargetID: "aws_security_group.web"},
			},
		},
	}

	g := Build(resources, nil)

	// Nodes
	if len(g.Nodes) != 6 {
		t.Fatalf("expected 6 nodes, got %d", len(g.Nodes))
	}

	nodeMap := make(map[string]models.Node)
	for _, n := range g.Nodes {
		nodeMap[n.ID] = n
	}

	assertNode := func(id, abstractType string, public bool) {
		t.Helper()
		n, ok := nodeMap[id]
		if !ok {
			t.Errorf("missing node %s", id)
			return
		}
		if n.Type != abstractType {
			t.Errorf("node %s: expected type %q, got %q", id, abstractType, n.Type)
		}
		if p, ok := n.Attributes["public"].(bool); !ok || p != public {
			t.Errorf("node %s: expected public=%v, got %v", id, public, n.Attributes["public"])
		}
	}

	assertNode("aws_vpc.main", "network", false)
	assertNode("aws_subnet.web", "network", false)
	assertNode("aws_security_group.web", "access_control", true)
	assertNode("aws_instance.web", "compute", true)
	assertNode("aws_lb.web", "entry_point", true)
	assertNode("aws_db_instance.main", "data", false)

	// Edges
	edgeSet := make(map[string]string)
	for _, e := range g.Edges {
		edgeSet[e.From+"|"+e.To] = e.Type
	}

	assertEdge := func(from, to, edgeType string) {
		t.Helper()
		got, ok := edgeSet[from+"|"+to]
		if !ok {
			t.Errorf("missing edge %s -> %s", from, to)
			return
		}
		if got != edgeType {
			t.Errorf("edge %s -> %s: expected type %q, got %q", from, to, edgeType, got)
		}
	}

	assertEdge("aws_subnet.web", "aws_vpc.main", "part_of")
	assertEdge("aws_security_group.web", "aws_vpc.main", "part_of")
	assertEdge("aws_instance.web", "aws_subnet.web", "deployed_in")
	assertEdge("aws_instance.web", "aws_security_group.web", "attached_to")
	assertEdge("aws_lb.web", "aws_security_group.web", "attached_to")
	assertEdge("aws_lb.web", "aws_subnet.web", "deployed_in")
	assertEdge("aws_db_instance.main", "aws_security_group.web", "attached_to")

	// Confidence
	if g.Confidence.Score != 1.0 {
		t.Errorf("expected confidence 1.0, got %f", g.Confidence.Score)
	}
}

func TestBuild_ConfidenceReduction(t *testing.T) {
	warnings := []models.Warning{
		{Category: models.WarnUnsupportedResource, Message: `unsupported resource type "aws_s3_bucket" (aws_s3_bucket.logs)`},
		{Category: models.WarnUnsupportedConstruct, Message: `module block "vpc" skipped (unsupported)`},
	}

	g := Build(nil, warnings)

	// 1.0 - 0.10 (unsupported_resource) - 0.05 (unsupported_construct) = 0.85
	if g.Confidence.Score > 0.9 {
		t.Errorf("expected reduced confidence, got %f", g.Confidence.Score)
	}
	if len(g.Confidence.Warnings) != 2 {
		t.Errorf("expected 2 warnings, got %d", len(g.Confidence.Warnings))
	}
}

func TestBuild_NilWarningsSerializesAsEmpty(t *testing.T) {
	g := Build(nil, nil)
	if g.Confidence.Warnings == nil {
		t.Fatal("expected non-nil warnings slice (must serialize as [], not null)")
	}
	if len(g.Confidence.Warnings) != 0 {
		t.Fatalf("expected empty warnings slice, got %d", len(g.Confidence.Warnings))
	}
}

func TestBuild_InfoWarningDoesNotReduceConfidence(t *testing.T) {
	warnings := []models.Warning{
		{Category: models.WarnInfo, Message: "no supported resources found in directory"},
	}
	g := Build(nil, warnings)
	if g.Confidence.Score != 1.0 {
		t.Errorf("info warning should not affect confidence, got %f", g.Confidence.Score)
	}
}

func TestBuild_EdgeDeduplication(t *testing.T) {
	resources := []models.RawResource{
		{
			Type: "aws_instance", Name: "web", ID: "aws_instance.web",
			Attributes: map[string]any{},
			References: []models.Reference{
				{SourceAttr: "vpc_security_group_ids", TargetID: "aws_security_group.web"},
				{SourceAttr: "security_groups", TargetID: "aws_security_group.web"},
			},
		},
		{
			Type: "aws_security_group", Name: "web", ID: "aws_security_group.web",
			Attributes: map[string]any{},
		},
	}

	g := Build(resources, nil)

	count := 0
	for _, e := range g.Edges {
		if e.From == "aws_instance.web" && e.To == "aws_security_group.web" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected 1 deduplicated edge, got %d", count)
	}
}
