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

// TestBuild_Tranche2_DerivedAttributesAndEdges locks in the Phase 7 PR4
// (v1.2) abstract types, public-attribute defaults, and new edge-type
// pairs introduced by the tranche-2 resource expansion.
func TestBuild_Tranche2_DerivedAttributesAndEdges(t *testing.T) {
	resources := []models.RawResource{
		{Type: "aws_vpc", Name: "main", ID: "aws_vpc.main"},
		{
			Type: "aws_subnet", Name: "main", ID: "aws_subnet.main",
			References: []models.Reference{{SourceAttr: "vpc_id", TargetID: "aws_vpc.main"}},
		},

		{
			Type: "aws_cloudfront_distribution", Name: "edge", ID: "aws_cloudfront_distribution.edge",
			Attributes: map[string]any{"web_acl_id": "arn:aws:wafv2:us-east-1:123:global/webacl/x/y"},
		},
		{Type: "aws_route53_zone", Name: "main", ID: "aws_route53_zone.main"},
		{
			Type: "aws_route53_record", Name: "www", ID: "aws_route53_record.www",
			References: []models.Reference{{SourceAttr: "zone_id", TargetID: "aws_route53_zone.main"}},
		},
		{Type: "aws_kms_key", Name: "main", ID: "aws_kms_key.main"},
		{
			Type: "aws_kms_alias", Name: "main", ID: "aws_kms_alias.main",
			References: []models.Reference{{SourceAttr: "target_key_id", TargetID: "aws_kms_key.main"}},
		},
		{Type: "aws_sns_topic", Name: "alerts", ID: "aws_sns_topic.alerts"},
		{
			Type: "aws_sns_topic_policy", Name: "alerts", ID: "aws_sns_topic_policy.alerts",
			Attributes: map[string]any{"policy": `{"Statement":[{"Effect":"Allow","Principal":"*"}]}`},
			References: []models.Reference{{SourceAttr: "arn", TargetID: "aws_sns_topic.alerts"}},
		},
		{Type: "aws_sqs_queue", Name: "jobs", ID: "aws_sqs_queue.jobs"},
		{
			Type: "aws_sqs_queue_policy", Name: "jobs", ID: "aws_sqs_queue_policy.jobs",
			References: []models.Reference{{SourceAttr: "queue_url", TargetID: "aws_sqs_queue.jobs"}},
		},
		{
			Type: "aws_nat_gateway", Name: "main", ID: "aws_nat_gateway.main",
			References: []models.Reference{{SourceAttr: "subnet_id", TargetID: "aws_subnet.main"}},
		},
		{
			Type: "aws_network_acl", Name: "main", ID: "aws_network_acl.main",
			References: []models.Reference{
				{SourceAttr: "vpc_id", TargetID: "aws_vpc.main"},
				{SourceAttr: "subnet_ids", TargetID: "aws_subnet.main"},
			},
		},
		{
			Type: "aws_network_acl_rule", Name: "open", ID: "aws_network_acl_rule.open",
			Attributes: map[string]any{
				"cidr_block":  "0.0.0.0/0",
				"egress":      false,
				"rule_action": "allow",
			},
			References: []models.Reference{{SourceAttr: "network_acl_id", TargetID: "aws_network_acl.main"}},
		},
		{Type: "aws_secretsmanager_secret", Name: "db", ID: "aws_secretsmanager_secret.db"},
		{
			Type: "aws_ebs_volume", Name: "data", ID: "aws_ebs_volume.data",
			Attributes: map[string]any{"encrypted": true},
		},
		{Type: "aws_ecs_cluster", Name: "main", ID: "aws_ecs_cluster.main"},
		{Type: "aws_ecs_task_definition", Name: "app", ID: "aws_ecs_task_definition.app"},
		{
			Type: "aws_ecs_service", Name: "app", ID: "aws_ecs_service.app",
			References: []models.Reference{
				{SourceAttr: "cluster", TargetID: "aws_ecs_cluster.main"},
				{SourceAttr: "task_definition", TargetID: "aws_ecs_task_definition.app"},
			},
		},
	}

	g := Build(resources, nil)

	nodeMap := make(map[string]models.Node, len(g.Nodes))
	for _, n := range g.Nodes {
		nodeMap[n.ID] = n
	}

	cases := []struct {
		id           string
		abstractType string
		public       bool
	}{
		{"aws_cloudfront_distribution.edge", "entry_point", true},
		{"aws_route53_zone.main", "network", false},
		{"aws_route53_record.www", "network", false},
		{"aws_kms_key.main", "identity", false},
		{"aws_kms_alias.main", "identity", false},
		{"aws_sns_topic.alerts", "data", false},
		{"aws_sns_topic_policy.alerts", "access_control", false},
		{"aws_sqs_queue.jobs", "data", false},
		{"aws_sqs_queue_policy.jobs", "access_control", false},
		{"aws_nat_gateway.main", "network", false},
		{"aws_network_acl.main", "access_control", false},
		{"aws_network_acl_rule.open", "access_control", false},
		{"aws_secretsmanager_secret.db", "data", false},
		{"aws_ebs_volume.data", "storage", false},
		{"aws_ecs_cluster.main", "compute", false},
		{"aws_ecs_task_definition.app", "compute", false},
		{"aws_ecs_service.app", "compute", false},
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

	// CloudFront's literal web_acl_id must be promoted so cloudfront_no_waf
	// can read it without re-parsing.
	cf := nodeMap["aws_cloudfront_distribution.edge"]
	if got, _ := cf.Attributes["web_acl_id"].(string); got == "" {
		t.Errorf("expected literal web_acl_id passthrough, got %v", cf.Attributes["web_acl_id"])
	}

	// Resolved SNS policy literal must reach the node so messaging_topic_public
	// can inspect Statement[].Effect.
	sns := nodeMap["aws_sns_topic_policy.alerts"]
	if got, _ := sns.Attributes["policy"].(string); got == "" {
		t.Errorf("expected SNS topic policy literal passthrough, got %v", sns.Attributes["policy"])
	}

	// EBS encrypted=true literal must be promoted so the rule can short-circuit.
	ebs := nodeMap["aws_ebs_volume.data"]
	if got, ok := ebs.Attributes["encrypted"].(bool); !ok || !got {
		t.Errorf("expected EBS encrypted=true passthrough, got %v", ebs.Attributes["encrypted"])
	}

	// NACL literals (cidr_block, egress, rule_action) must all promote.
	nacl := nodeMap["aws_network_acl_rule.open"]
	if got, _ := nacl.Attributes["cidr_block"].(string); got != "0.0.0.0/0" {
		t.Errorf("NACL cidr_block passthrough = %v", nacl.Attributes["cidr_block"])
	}
	if got, ok := nacl.Attributes["egress"].(bool); !ok || got {
		t.Errorf("NACL egress passthrough = %v", nacl.Attributes["egress"])
	}
	if got, _ := nacl.Attributes["rule_action"].(string); got != "allow" {
		t.Errorf("NACL rule_action passthrough = %v", nacl.Attributes["rule_action"])
	}

	edgeSet := make(map[string]string, len(g.Edges))
	for _, e := range g.Edges {
		edgeSet[e.From+"|"+e.To] = e.Type
	}
	edgeCases := []struct {
		from, to, edgeType string
	}{
		{"aws_route53_record.www", "aws_route53_zone.main", "part_of"},
		{"aws_kms_alias.main", "aws_kms_key.main", "applies_to"},
		{"aws_sns_topic_policy.alerts", "aws_sns_topic.alerts", "applies_to"},
		{"aws_sqs_queue_policy.jobs", "aws_sqs_queue.jobs", "applies_to"},
		{"aws_nat_gateway.main", "aws_subnet.main", "deployed_in"},
		{"aws_network_acl.main", "aws_vpc.main", "part_of"},
		{"aws_network_acl.main", "aws_subnet.main", "applies_to"},
		{"aws_network_acl_rule.open", "aws_network_acl.main", "applies_to"},
		{"aws_ecs_service.app", "aws_ecs_cluster.main", "deployed_in"},
		{"aws_ecs_service.app", "aws_ecs_task_definition.app", "uses"},
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

	if g.Confidence.Score != 1.0 {
		t.Errorf("tranche-2 fixture must produce confidence 1.0 (no warnings), got %f", g.Confidence.Score)
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
