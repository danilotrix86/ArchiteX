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

// TestBuild_Phase6_DerivedAttributesAndEdges locks in the Phase 6 (v1.1)
// public-attribute defaults and the new edge-type pairs. If you add or
// reshuffle Phase 6 resources, this test is the contract that downstream
// rules and the Mermaid renderer rely on.
func TestBuild_Phase6_DerivedAttributesAndEdges(t *testing.T) {
	resources := []models.RawResource{
		// Storage + governance siblings.
		{Type: "aws_s3_bucket", Name: "logs", ID: "aws_s3_bucket.logs"},
		{
			Type: "aws_s3_bucket_public_access_block", Name: "logs", ID: "aws_s3_bucket_public_access_block.logs",
			References: []models.Reference{{SourceAttr: "bucket", TargetID: "aws_s3_bucket.logs"}},
		},
		{
			Type: "aws_s3_bucket_policy", Name: "logs", ID: "aws_s3_bucket_policy.logs",
			References: []models.Reference{{SourceAttr: "bucket", TargetID: "aws_s3_bucket.logs"}},
		},

		// Identity stack.
		{Type: "aws_iam_role", Name: "app", ID: "aws_iam_role.app"},
		{Type: "aws_iam_policy", Name: "read", ID: "aws_iam_policy.read"},
		{
			Type: "aws_iam_role_policy_attachment", Name: "app_read", ID: "aws_iam_role_policy_attachment.app_read",
			References: []models.Reference{
				{SourceAttr: "role", TargetID: "aws_iam_role.app"},
				{SourceAttr: "policy_arn", TargetID: "aws_iam_policy.read"},
			},
		},

		// Lambda + URL + execution role wiring.
		{
			Type: "aws_lambda_function", Name: "worker", ID: "aws_lambda_function.worker",
			References: []models.Reference{{SourceAttr: "role", TargetID: "aws_iam_role.app"}},
		},
		{
			Type: "aws_lambda_function_url", Name: "worker", ID: "aws_lambda_function_url.worker",
			References: []models.Reference{{SourceAttr: "function_name", TargetID: "aws_lambda_function.worker"}},
		},

		// Inherently-public Phase 6 nodes.
		{Type: "aws_apigatewayv2_api", Name: "http", ID: "aws_apigatewayv2_api.http"},
		{Type: "aws_vpc", Name: "main", ID: "aws_vpc.main"},
		{
			Type: "aws_internet_gateway", Name: "main", ID: "aws_internet_gateway.main",
			References: []models.Reference{{SourceAttr: "vpc_id", TargetID: "aws_vpc.main"}},
		},
	}

	g := Build(resources, nil)

	nodeMap := make(map[string]models.Node, len(g.Nodes))
	for _, n := range g.Nodes {
		nodeMap[n.ID] = n
	}

	// --- abstract type + public-attribute contract ---
	cases := []struct {
		id           string
		abstractType string
		public       bool
	}{
		{"aws_s3_bucket.logs", "storage", false},
		{"aws_s3_bucket_public_access_block.logs", "access_control", false},
		{"aws_s3_bucket_policy.logs", "access_control", false},
		{"aws_iam_role.app", "identity", false},
		{"aws_iam_policy.read", "identity", false},
		{"aws_iam_role_policy_attachment.app_read", "identity", false},
		{"aws_lambda_function.worker", "compute", false},
		{"aws_lambda_function_url.worker", "entry_point", true},
		{"aws_apigatewayv2_api.http", "entry_point", true},
		{"aws_internet_gateway.main", "network", true},
	}
	for _, c := range cases {
		n, ok := nodeMap[c.id]
		if !ok {
			t.Errorf("missing node %s", c.id)
			continue
		}
		if n.Type != c.abstractType {
			t.Errorf("node %s: expected abstract type %q, got %q", c.id, c.abstractType, n.Type)
		}
		if got, _ := n.Attributes["public"].(bool); got != c.public {
			t.Errorf("node %s: expected public=%v, got %v", c.id, c.public, n.Attributes["public"])
		}
	}

	// --- edge-type contract for the new pairs ---
	edgeSet := make(map[string]string, len(g.Edges))
	for _, e := range g.Edges {
		edgeSet[e.From+"|"+e.To] = e.Type
	}
	edgeCases := []struct {
		from, to, edgeType string
	}{
		{"aws_s3_bucket_public_access_block.logs", "aws_s3_bucket.logs", "applies_to"},
		{"aws_s3_bucket_policy.logs", "aws_s3_bucket.logs", "applies_to"},
		{"aws_iam_role_policy_attachment.app_read", "aws_iam_role.app", "applies_to"},
		{"aws_iam_role_policy_attachment.app_read", "aws_iam_policy.read", "applies_to"},
		{"aws_lambda_function.worker", "aws_iam_role.app", "attached_to"},
		{"aws_lambda_function_url.worker", "aws_lambda_function.worker", "applies_to"},
		{"aws_internet_gateway.main", "aws_vpc.main", "part_of"},
	}
	for _, c := range edgeCases {
		got, ok := edgeSet[c.from+"|"+c.to]
		if !ok {
			t.Errorf("missing edge %s -> %s", c.from, c.to)
			continue
		}
		if got != c.edgeType {
			t.Errorf("edge %s -> %s: expected type %q, got %q", c.from, c.to, c.edgeType, got)
		}
	}

	// --- confidence must remain pristine: every Phase 6 type is now first-class ---
	if g.Confidence.Score != 1.0 {
		t.Errorf("Phase 6 fixture must produce confidence 1.0 (no warnings), got %f", g.Confidence.Score)
	}
}

// TestBuild_Phase6_IAMAttachment_PolicyARNPassthrough is the contract test
// that lets the iam_admin_policy_attached risk rule (Phase 6 PR2) inspect a
// literal policy_arn at the graph node level. If this passthrough breaks,
// the rule will silently stop firing on AdministratorAccess attachments --
// catastrophic for the v1.1 detection promise.
func TestBuild_Phase6_IAMAttachment_PolicyARNPassthrough(t *testing.T) {
	resources := []models.RawResource{
		{Type: "aws_iam_role", Name: "app", ID: "aws_iam_role.app"},
		{
			Type: "aws_iam_role_policy_attachment", Name: "admin",
			ID: "aws_iam_role_policy_attachment.admin",
			Attributes: map[string]any{
				"policy_arn": "arn:aws:iam::aws:policy/AdministratorAccess",
			},
			References: []models.Reference{
				{SourceAttr: "role", TargetID: "aws_iam_role.app"},
			},
		},
		{
			Type: "aws_iam_role_policy_attachment", Name: "dynamic",
			ID: "aws_iam_role_policy_attachment.dynamic",
			// nil here mirrors what extract.go writes when policy_arn is a
			// variable / cross-resource reference rather than a literal.
			Attributes: map[string]any{"policy_arn": nil},
			References: []models.Reference{
				{SourceAttr: "role", TargetID: "aws_iam_role.app"},
			},
		},
	}

	g := Build(resources, nil)

	var admin, dynamic models.Node
	for _, n := range g.Nodes {
		switch n.ID {
		case "aws_iam_role_policy_attachment.admin":
			admin = n
		case "aws_iam_role_policy_attachment.dynamic":
			dynamic = n
		}
	}

	if got, _ := admin.Attributes["policy_arn"].(string); got != "arn:aws:iam::aws:policy/AdministratorAccess" {
		t.Errorf("admin attachment: expected policy_arn passthrough as literal string, got %v", admin.Attributes["policy_arn"])
	}
	if _, exists := dynamic.Attributes["policy_arn"]; exists {
		t.Errorf("dynamic attachment: unresolved policy_arn must NOT be promoted to the graph node, got %v", dynamic.Attributes["policy_arn"])
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
