package risk

import (
	"testing"

	"architex/delta"
	"architex/models"
)

// ---------------------------------------------------------------------------
// eks_public_endpoint
// ---------------------------------------------------------------------------

func TestEvaluate_EKS_PublicEndpoint_Fires(t *testing.T) {
	d := delta.Delta{
		AddedNodes: []models.Node{
			{
				ID:           "aws_eks_cluster.open",
				Type:         "compute",
				ProviderType: "aws_eks_cluster",
				Attributes: map[string]any{
					"public":                 true,
					"endpoint_public_access": true,
				},
			},
		},
		Summary: delta.DeltaSummary{AddedNodes: 1},
	}
	r := Evaluate(d)
	if !hasReason(r.Reasons, "eks_public_endpoint") {
		t.Fatalf("expected eks_public_endpoint, got %+v", r.Reasons)
	}
}

func TestEvaluate_EKS_PublicEndpoint_WithCIDRAllowList_Suppressed(t *testing.T) {
	d := delta.Delta{
		AddedNodes: []models.Node{
			{
				ID:           "aws_eks_cluster.scoped",
				Type:         "compute",
				ProviderType: "aws_eks_cluster",
				Attributes: map[string]any{
					"public":                       true,
					"endpoint_public_access":       true,
					"endpoint_public_access_cidrs": []any{"203.0.113.0/24"},
				},
			},
		},
		Summary: delta.DeltaSummary{AddedNodes: 1},
	}
	r := Evaluate(d)
	if hasReason(r.Reasons, "eks_public_endpoint") {
		t.Fatalf("rule must NOT fire when an allow-list is present")
	}
}

func TestEvaluate_EKS_PrivateEndpoint_DoesNotFire(t *testing.T) {
	d := delta.Delta{
		AddedNodes: []models.Node{
			{
				ID:           "aws_eks_cluster.private",
				Type:         "compute",
				ProviderType: "aws_eks_cluster",
				Attributes: map[string]any{
					"public":                 false,
					"endpoint_public_access": false,
				},
			},
		},
		Summary: delta.DeltaSummary{AddedNodes: 1},
	}
	r := Evaluate(d)
	if hasReason(r.Reasons, "eks_public_endpoint") {
		t.Fatalf("rule must NOT fire when endpoint is private")
	}
}

func TestEvaluate_EKS_UnresolvedEndpoint_DoesNotFire(t *testing.T) {
	// Variable-driven `endpoint_public_access = var.public` lands as
	// missing in graph.deriveAttributes (only literal bools promote).
	// Rule MUST stay silent (consistent with "never guess").
	d := delta.Delta{
		AddedNodes: []models.Node{
			{
				ID:           "aws_eks_cluster.unresolved",
				Type:         "compute",
				ProviderType: "aws_eks_cluster",
				Attributes:   map[string]any{"public": false},
			},
		},
		Summary: delta.DeltaSummary{AddedNodes: 1},
	}
	r := Evaluate(d)
	if hasReason(r.Reasons, "eks_public_endpoint") {
		t.Fatalf("rule must NOT fire on unresolved endpoint config")
	}
}

// ---------------------------------------------------------------------------
// eks_no_logging
// ---------------------------------------------------------------------------

func TestEvaluate_EKS_NoLogging_Fires(t *testing.T) {
	d := delta.Delta{
		AddedNodes: []models.Node{
			{
				ID:           "aws_eks_cluster.silent",
				Type:         "compute",
				ProviderType: "aws_eks_cluster",
				Attributes: map[string]any{
					"public":                 false,
					"endpoint_public_access": false,
				},
			},
		},
		Summary: delta.DeltaSummary{AddedNodes: 1},
	}
	r := Evaluate(d)
	if !hasReason(r.Reasons, "eks_no_logging") {
		t.Fatalf("expected eks_no_logging, got %+v", r.Reasons)
	}
}

func TestEvaluate_EKS_WithLogging_Suppressed(t *testing.T) {
	d := delta.Delta{
		AddedNodes: []models.Node{
			{
				ID:           "aws_eks_cluster.logged",
				Type:         "compute",
				ProviderType: "aws_eks_cluster",
				Attributes: map[string]any{
					"public":                    false,
					"endpoint_public_access":    false,
					"enabled_cluster_log_types": []any{"api", "audit"},
				},
			},
		},
		Summary: delta.DeltaSummary{AddedNodes: 1},
	}
	r := Evaluate(d)
	if hasReason(r.Reasons, "eks_no_logging") {
		t.Fatalf("rule must NOT fire when log types are enabled")
	}
}

// ---------------------------------------------------------------------------
// asg_unrestricted_scaling
// ---------------------------------------------------------------------------

func TestEvaluate_ASG_UnrestrictedScaling_Fires(t *testing.T) {
	d := delta.Delta{
		AddedNodes: []models.Node{
			{
				ID:           "aws_autoscaling_group.runaway",
				Type:         "compute",
				ProviderType: "aws_autoscaling_group",
				Attributes: map[string]any{
					"public":   false,
					"max_size": float64(250),
					"min_size": float64(0),
				},
			},
		},
		Summary: delta.DeltaSummary{AddedNodes: 1},
	}
	r := Evaluate(d)
	if !hasReason(r.Reasons, "asg_unrestricted_scaling") {
		t.Fatalf("expected asg_unrestricted_scaling, got %+v", r.Reasons)
	}
}

func TestEvaluate_ASG_BoundedMaxSize_DoesNotFire(t *testing.T) {
	d := delta.Delta{
		AddedNodes: []models.Node{
			{
				ID:           "aws_autoscaling_group.small",
				Type:         "compute",
				ProviderType: "aws_autoscaling_group",
				Attributes: map[string]any{
					"public":   false,
					"max_size": float64(50),
					"min_size": float64(1),
				},
			},
		},
		Summary: delta.DeltaSummary{AddedNodes: 1},
	}
	r := Evaluate(d)
	if hasReason(r.Reasons, "asg_unrestricted_scaling") {
		t.Fatalf("rule must NOT fire when max_size <= threshold")
	}
}

func TestEvaluate_ASG_WithMinFloor_DoesNotFire(t *testing.T) {
	d := delta.Delta{
		AddedNodes: []models.Node{
			{
				ID:           "aws_autoscaling_group.floored",
				Type:         "compute",
				ProviderType: "aws_autoscaling_group",
				Attributes: map[string]any{
					"public":   false,
					"max_size": float64(500),
					"min_size": float64(10),
				},
			},
		},
		Summary: delta.DeltaSummary{AddedNodes: 1},
	}
	r := Evaluate(d)
	if hasReason(r.Reasons, "asg_unrestricted_scaling") {
		t.Fatalf("rule must NOT fire when min_size > 0 floors the ASG")
	}
}

func TestEvaluate_ASG_UnresolvedMaxSize_DoesNotFire(t *testing.T) {
	// Variable-driven `max_size = var.max_capacity` lands as missing
	// (only float64 literals are promoted). Rule MUST stay silent.
	d := delta.Delta{
		AddedNodes: []models.Node{
			{
				ID:           "aws_autoscaling_group.unresolved",
				Type:         "compute",
				ProviderType: "aws_autoscaling_group",
				Attributes:   map[string]any{"public": false},
			},
		},
		Summary: delta.DeltaSummary{AddedNodes: 1},
	}
	r := Evaluate(d)
	if hasReason(r.Reasons, "asg_unrestricted_scaling") {
		t.Fatalf("rule must NOT fire when max_size is unresolved")
	}
}

// ---------------------------------------------------------------------------
// Conditional-node guard (PR2 contract; the helper is owned by PR1).
// ---------------------------------------------------------------------------

// TestEvaluate_ConditionalNodes_NeverScored is the lock-in regression for
// the v1.3 library-mode guard: a node carrying Attributes["conditional"]
// = true is a parser-materialized phantom. No risk rule may score it,
// because the resource's existence itself is gated on a variable. This
// preserves the engine invariant "never invent findings on resources
// whose existence is conditional".
func TestEvaluate_ConditionalNodes_NeverScored(t *testing.T) {
	policy := `{"Statement":[{"Effect":"Allow","Principal":"*","Action":"*"}]}`
	d := delta.Delta{
		AddedNodes: []models.Node{
			{
				ID:           "aws_eks_cluster.maybe",
				Type:         "compute",
				ProviderType: "aws_eks_cluster",
				Attributes: map[string]any{
					"public":                 true,
					"endpoint_public_access": true,
					"conditional":            true,
				},
			},
			{
				ID:           "aws_autoscaling_group.maybe",
				Type:         "compute",
				ProviderType: "aws_autoscaling_group",
				Attributes: map[string]any{
					"public":      false,
					"max_size":    float64(500),
					"min_size":    float64(0),
					"conditional": true,
				},
			},
			{
				ID:           "aws_iam_role_policy_attachment.maybe",
				Type:         "identity",
				ProviderType: "aws_iam_role_policy_attachment",
				Attributes: map[string]any{
					"policy_arn":  "arn:aws:iam::aws:policy/AdministratorAccess",
					"conditional": true,
				},
			},
			{
				ID:           "aws_lambda_function_url.maybe",
				Type:         "entry_point",
				ProviderType: "aws_lambda_function_url",
				Attributes: map[string]any{
					"public":      true,
					"conditional": true,
				},
			},
			{
				ID:           "aws_sns_topic_policy.maybe",
				Type:         "access_control",
				ProviderType: "aws_sns_topic_policy",
				Attributes: map[string]any{
					"policy":      policy,
					"conditional": true,
				},
			},
			{
				ID:           "aws_cloudfront_distribution.maybe",
				Type:         "entry_point",
				ProviderType: "aws_cloudfront_distribution",
				Attributes: map[string]any{
					"public":      true,
					"conditional": true,
				},
			},
			{
				ID:           "aws_ebs_volume.maybe",
				Type:         "storage",
				ProviderType: "aws_ebs_volume",
				Attributes: map[string]any{
					"public":      false,
					"encrypted":   false,
					"conditional": true,
				},
			},
			{
				ID:           "aws_network_acl_rule.maybe",
				Type:         "access_control",
				ProviderType: "aws_network_acl_rule",
				Attributes: map[string]any{
					"public":      false,
					"cidr_block":  "0.0.0.0/0",
					"egress":      false,
					"rule_action": "allow",
					"conditional": true,
				},
			},
		},
		Summary: delta.DeltaSummary{AddedNodes: 8},
	}
	r := Evaluate(d)
	for _, reason := range r.Reasons {
		// "new_entry_point", "new_data_resource", and "resource_removed"
		// are added-count rules that don't touch attributes; for now
		// they're allowed to fire on phantoms because the diagram still
		// surfaces them as "?". The substantive scoring rules
		// (eks_*, asg_*, iam_admin_*, lambda_public_url_*,
		// messaging_*, cloudfront_*, ebs_*, nacl_*) MUST stay silent.
		switch reason.RuleID {
		case "eks_public_endpoint",
			"eks_no_logging",
			"asg_unrestricted_scaling",
			"iam_admin_policy_attached",
			"lambda_public_url_introduced",
			"messaging_topic_public",
			"cloudfront_no_waf",
			"ebs_volume_unencrypted",
			"nacl_allow_all_ingress":
			t.Errorf("rule %q must not fire on conditional=true node %s",
				reason.RuleID, reason.ResourceID)
		}
	}
}
