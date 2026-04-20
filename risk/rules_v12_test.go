package risk

import (
	"strings"
	"testing"

	"architex/delta"
	"architex/models"
)

// ---------------------------------------------------------------------------
// cloudfront_no_waf
// ---------------------------------------------------------------------------

func TestEvaluate_CloudFront_NoWAF_Fires(t *testing.T) {
	d := delta.Delta{
		AddedNodes: []models.Node{
			{
				ID:           "aws_cloudfront_distribution.web",
				Type:         "entry_point",
				ProviderType: "aws_cloudfront_distribution",
				Attributes:   map[string]any{"public": true},
			},
		},
		Summary: delta.DeltaSummary{AddedNodes: 1},
	}
	r := Evaluate(d)
	if !hasReason(r.Reasons, "cloudfront_no_waf") {
		t.Fatalf("expected cloudfront_no_waf, got %+v", r.Reasons)
	}
	// Stacks with new_entry_point.
	if !hasReason(r.Reasons, "new_entry_point") {
		t.Fatalf("expected new_entry_point to also fire on CF distro add")
	}
}

func TestEvaluate_CloudFront_WithWAF_Suppressed(t *testing.T) {
	d := delta.Delta{
		AddedNodes: []models.Node{
			{
				ID:           "aws_cloudfront_distribution.web",
				Type:         "entry_point",
				ProviderType: "aws_cloudfront_distribution",
				Attributes: map[string]any{
					"public":     true,
					"web_acl_id": "arn:aws:wafv2:us-east-1:123:global/webacl/edge/abcd",
				},
			},
		},
		Summary: delta.DeltaSummary{AddedNodes: 1},
	}
	r := Evaluate(d)
	if hasReason(r.Reasons, "cloudfront_no_waf") {
		t.Fatalf("cloudfront_no_waf must NOT fire when web_acl_id is set; got %+v", r.Reasons)
	}
}

func TestEvaluate_CloudFront_VarDriven_FiresConservatively(t *testing.T) {
	// Variable-driven attachment lands as missing in graph.deriveAttributes
	// (only literal strings are promoted). Rule should fire conservatively.
	d := delta.Delta{
		AddedNodes: []models.Node{
			{
				ID:           "aws_cloudfront_distribution.web",
				Type:         "entry_point",
				ProviderType: "aws_cloudfront_distribution",
				Attributes:   map[string]any{"public": true},
			},
		},
		Summary: delta.DeltaSummary{AddedNodes: 1},
	}
	r := Evaluate(d)
	if !hasReason(r.Reasons, "cloudfront_no_waf") {
		t.Fatal("cloudfront_no_waf must fire when web_acl_id is unresolved")
	}
}

// ---------------------------------------------------------------------------
// ebs_volume_unencrypted
// ---------------------------------------------------------------------------

func TestEvaluate_EBS_Unencrypted_Fires(t *testing.T) {
	d := delta.Delta{
		AddedNodes: []models.Node{
			{
				ID:           "aws_ebs_volume.data",
				Type:         "storage",
				ProviderType: "aws_ebs_volume",
				Attributes:   map[string]any{"public": false, "encrypted": false},
			},
		},
		Summary: delta.DeltaSummary{AddedNodes: 1},
	}
	r := Evaluate(d)
	if !hasReason(r.Reasons, "ebs_volume_unencrypted") {
		t.Fatalf("expected ebs_volume_unencrypted, got %+v", r.Reasons)
	}
}

func TestEvaluate_EBS_Encrypted_Suppressed(t *testing.T) {
	d := delta.Delta{
		AddedNodes: []models.Node{
			{
				ID:           "aws_ebs_volume.data",
				Type:         "storage",
				ProviderType: "aws_ebs_volume",
				Attributes:   map[string]any{"public": false, "encrypted": true},
			},
		},
		Summary: delta.DeltaSummary{AddedNodes: 1},
	}
	r := Evaluate(d)
	if hasReason(r.Reasons, "ebs_volume_unencrypted") {
		t.Fatalf("rule must NOT fire when encrypted=true")
	}
}

func TestEvaluate_EBS_MissingEncrypted_DoesNotFire(t *testing.T) {
	// Variable-driven `encrypted = var.foo` lands as missing in the graph;
	// account-level encryption-by-default may still encrypt at runtime, so
	// the rule stays silent (no false positive).
	d := delta.Delta{
		AddedNodes: []models.Node{
			{
				ID:           "aws_ebs_volume.data",
				Type:         "storage",
				ProviderType: "aws_ebs_volume",
				Attributes:   map[string]any{"public": false},
			},
		},
		Summary: delta.DeltaSummary{AddedNodes: 1},
	}
	r := Evaluate(d)
	if hasReason(r.Reasons, "ebs_volume_unencrypted") {
		t.Fatalf("rule must stay silent when `encrypted` is unresolved")
	}
}

// ---------------------------------------------------------------------------
// messaging_topic_public
// ---------------------------------------------------------------------------

func TestEvaluate_SNS_Public_Fires(t *testing.T) {
	policy := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":"*","Action":"sns:Publish","Resource":"*"}]}`
	d := delta.Delta{
		AddedNodes: []models.Node{
			{
				ID:           "aws_sns_topic_policy.public",
				Type:         "access_control",
				ProviderType: "aws_sns_topic_policy",
				Attributes:   map[string]any{"public": false, "policy": policy},
			},
		},
		Summary: delta.DeltaSummary{AddedNodes: 1},
	}
	r := Evaluate(d)
	if !hasReason(r.Reasons, "messaging_topic_public") {
		t.Fatalf("expected messaging_topic_public, got %+v", r.Reasons)
	}
}

func TestEvaluate_SQS_PublicAWSWildcard_Fires(t *testing.T) {
	policy := `{"Statement":[{"Effect":"Allow","Principal":{"AWS":"*"},"Action":"sqs:SendMessage","Resource":"*"}]}`
	d := delta.Delta{
		AddedNodes: []models.Node{
			{
				ID:           "aws_sqs_queue_policy.public",
				Type:         "access_control",
				ProviderType: "aws_sqs_queue_policy",
				Attributes:   map[string]any{"public": false, "policy": policy},
			},
		},
		Summary: delta.DeltaSummary{AddedNodes: 1},
	}
	r := Evaluate(d)
	if !hasReason(r.Reasons, "messaging_topic_public") {
		t.Fatalf("expected messaging_topic_public on SQS, got %+v", r.Reasons)
	}
}

func TestEvaluate_SNS_DenyOnly_Suppressed(t *testing.T) {
	policy := `{"Statement":[{"Effect":"Deny","Principal":"*","Action":"sns:Publish","Resource":"*"}]}`
	d := delta.Delta{
		AddedNodes: []models.Node{
			{
				ID:           "aws_sns_topic_policy.locked",
				Type:         "access_control",
				ProviderType: "aws_sns_topic_policy",
				Attributes:   map[string]any{"public": false, "policy": policy},
			},
		},
		Summary: delta.DeltaSummary{AddedNodes: 1},
	}
	r := Evaluate(d)
	if hasReason(r.Reasons, "messaging_topic_public") {
		t.Fatalf("rule must NOT fire on Deny statements")
	}
}

func TestEvaluate_SNS_ScopedPrincipal_Suppressed(t *testing.T) {
	policy := `{"Statement":[{"Effect":"Allow","Principal":{"AWS":"arn:aws:iam::123:role/app"},"Action":"sns:Publish"}]}`
	d := delta.Delta{
		AddedNodes: []models.Node{
			{
				ID:           "aws_sns_topic_policy.scoped",
				Type:         "access_control",
				ProviderType: "aws_sns_topic_policy",
				Attributes:   map[string]any{"public": false, "policy": policy},
			},
		},
		Summary: delta.DeltaSummary{AddedNodes: 1},
	}
	r := Evaluate(d)
	if hasReason(r.Reasons, "messaging_topic_public") {
		t.Fatalf("rule must NOT fire on scoped principals")
	}
}

func TestEvaluate_SNS_UnresolvedPolicy_DoesNotFire(t *testing.T) {
	// Variable-driven policy lands as nil in the graph: rule MUST stay
	// silent (no guessing). This is symmetric with iam_admin_policy_attached.
	d := delta.Delta{
		AddedNodes: []models.Node{
			{
				ID:           "aws_sns_topic_policy.unresolved",
				Type:         "access_control",
				ProviderType: "aws_sns_topic_policy",
				Attributes:   map[string]any{"public": false},
			},
		},
		Summary: delta.DeltaSummary{AddedNodes: 1},
	}
	r := Evaluate(d)
	if hasReason(r.Reasons, "messaging_topic_public") {
		t.Fatalf("rule must NOT fire when policy is unresolved")
	}
}

// ---------------------------------------------------------------------------
// nacl_allow_all_ingress
// ---------------------------------------------------------------------------

func TestEvaluate_NACL_AllowAllIngress_Fires(t *testing.T) {
	d := delta.Delta{
		AddedNodes: []models.Node{
			{
				ID:           "aws_network_acl_rule.open",
				Type:         "access_control",
				ProviderType: "aws_network_acl_rule",
				Attributes: map[string]any{
					"public":      false,
					"cidr_block":  "0.0.0.0/0",
					"egress":      false,
					"rule_action": "allow",
				},
			},
		},
		Summary: delta.DeltaSummary{AddedNodes: 1},
	}
	r := Evaluate(d)
	if !hasReason(r.Reasons, "nacl_allow_all_ingress") {
		t.Fatalf("expected nacl_allow_all_ingress, got %+v", r.Reasons)
	}
}

func TestEvaluate_NACL_Egress_DoesNotFire(t *testing.T) {
	d := delta.Delta{
		AddedNodes: []models.Node{
			{
				ID:           "aws_network_acl_rule.egress",
				Type:         "access_control",
				ProviderType: "aws_network_acl_rule",
				Attributes: map[string]any{
					"public":      false,
					"cidr_block":  "0.0.0.0/0",
					"egress":      true,
					"rule_action": "allow",
				},
			},
		},
		Summary: delta.DeltaSummary{AddedNodes: 1},
	}
	r := Evaluate(d)
	if hasReason(r.Reasons, "nacl_allow_all_ingress") {
		t.Fatalf("rule must NOT fire for egress=true")
	}
}

func TestEvaluate_NACL_DenyAction_DoesNotFire(t *testing.T) {
	d := delta.Delta{
		AddedNodes: []models.Node{
			{
				ID:           "aws_network_acl_rule.deny",
				Type:         "access_control",
				ProviderType: "aws_network_acl_rule",
				Attributes: map[string]any{
					"public":      false,
					"cidr_block":  "0.0.0.0/0",
					"egress":      false,
					"rule_action": "deny",
				},
			},
		},
		Summary: delta.DeltaSummary{AddedNodes: 1},
	}
	r := Evaluate(d)
	if hasReason(r.Reasons, "nacl_allow_all_ingress") {
		t.Fatalf("rule must NOT fire for action=deny")
	}
}

func TestEvaluate_NACL_ScopedCIDR_DoesNotFire(t *testing.T) {
	d := delta.Delta{
		AddedNodes: []models.Node{
			{
				ID:           "aws_network_acl_rule.scoped",
				Type:         "access_control",
				ProviderType: "aws_network_acl_rule",
				Attributes: map[string]any{
					"public":      false,
					"cidr_block":  "10.0.0.0/8",
					"egress":      false,
					"rule_action": "allow",
				},
			},
		},
		Summary: delta.DeltaSummary{AddedNodes: 1},
	}
	r := Evaluate(d)
	if hasReason(r.Reasons, "nacl_allow_all_ingress") {
		t.Fatalf("rule must NOT fire for non-0.0.0.0/0 cidr")
	}
}

// ---------------------------------------------------------------------------
// helper
// ---------------------------------------------------------------------------

func hasReason(reasons []RiskReason, ruleID string) bool {
	for _, r := range reasons {
		if r.RuleID == ruleID {
			return true
		}
	}
	return false
}

func TestEvaluate_Tranche2_StackingScoresHigh(t *testing.T) {
	policy := `{"Statement":[{"Effect":"Allow","Principal":"*","Action":"*"}]}`
	d := delta.Delta{
		AddedNodes: []models.Node{
			{
				ID:           "aws_cloudfront_distribution.web",
				Type:         "entry_point",
				ProviderType: "aws_cloudfront_distribution",
				Attributes:   map[string]any{"public": true},
			},
			{
				ID:           "aws_sns_topic_policy.public",
				Type:         "access_control",
				ProviderType: "aws_sns_topic_policy",
				Attributes:   map[string]any{"public": false, "policy": policy},
			},
		},
		Summary: delta.DeltaSummary{AddedNodes: 2},
	}
	r := Evaluate(d)
	if r.Score < 7.0 {
		t.Fatalf("expected combined tranche2 fixture to land in fail tier, got %.1f", r.Score)
	}
	wantStatus := "fail"
	if r.Status != wantStatus {
		t.Fatalf("expected status=%s, got %s\nreasons:\n%s",
			wantStatus, r.Status, summarizeReasons(r.Reasons))
	}
}

func summarizeReasons(reasons []RiskReason) string {
	var b strings.Builder
	for _, r := range reasons {
		b.WriteString("  - ")
		b.WriteString(r.RuleID)
		b.WriteString(": ")
		b.WriteString(r.Message)
		b.WriteString("\n")
	}
	return b.String()
}
