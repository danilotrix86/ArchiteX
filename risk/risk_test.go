package risk

import (
	"fmt"
	"testing"

	"architex/delta"
	"architex/models"
)

func TestEvaluate_PublicExposure(t *testing.T) {
	d := delta.Delta{
		ChangedNodes: []delta.ChangedNode{
			{
				ID:           "aws_security_group.web",
				Type:         "access_control",
				ProviderType: "aws_security_group",
				ChangedAttributes: map[string]delta.ChangedAttribute{
					"public": {Before: false, After: true},
				},
			},
		},
		Summary: delta.DeltaSummary{ChangedNodes: 1},
	}

	r := Evaluate(d)

	// Rule 1 (4.0) + Rule 4 (2.0): SG is access_control, so data-exposure also fires.
	if r.Score != 6.0 {
		t.Fatalf("expected score 6.0, got %.1f", r.Score)
	}
	if r.Severity != "medium" {
		t.Fatalf("expected severity medium, got %s", r.Severity)
	}
	if r.Status != "warn" {
		t.Fatalf("expected status warn, got %s", r.Status)
	}
	if len(r.Reasons) != 2 {
		t.Fatalf("expected 2 reasons, got %d", len(r.Reasons))
	}
	if r.Reasons[0].RuleID != "public_exposure_introduced" {
		t.Fatalf("expected first reason public_exposure_introduced, got %s", r.Reasons[0].RuleID)
	}
	if r.Reasons[1].RuleID != "potential_data_exposure" {
		t.Fatalf("expected second reason potential_data_exposure, got %s", r.Reasons[1].RuleID)
	}
}

func TestEvaluate_NewDataOnly(t *testing.T) {
	d := delta.Delta{
		AddedNodes: []models.Node{
			{ID: "aws_db_instance.main", Type: "data", ProviderType: "aws_db_instance", Attributes: map[string]any{"public": false}},
		},
		Summary: delta.DeltaSummary{AddedNodes: 1},
	}

	r := Evaluate(d)

	if r.Score != 2.5 {
		t.Fatalf("expected score 2.5, got %.1f", r.Score)
	}
	if r.Severity != "low" {
		t.Fatalf("expected severity low, got %s", r.Severity)
	}
	if r.Status != "pass" {
		t.Fatalf("expected status pass, got %s", r.Status)
	}
	if len(r.Reasons) != 1 {
		t.Fatalf("expected 1 reason, got %d", len(r.Reasons))
	}
	if r.Reasons[0].RuleID != "new_data_resource" {
		t.Fatalf("expected rule new_data_resource, got %s", r.Reasons[0].RuleID)
	}
}

func TestEvaluate_PublicPlusDB(t *testing.T) {
	d := delta.Delta{
		AddedNodes: []models.Node{
			{ID: "aws_db_instance.main", Type: "data", ProviderType: "aws_db_instance", Attributes: map[string]any{"public": false}},
		},
		ChangedNodes: []delta.ChangedNode{
			{
				ID:           "aws_security_group.web",
				Type:         "access_control",
				ProviderType: "aws_security_group",
				ChangedAttributes: map[string]delta.ChangedAttribute{
					"public": {Before: false, After: true},
				},
			},
		},
		Summary: delta.DeltaSummary{AddedNodes: 1, ChangedNodes: 1},
	}

	r := Evaluate(d)

	// Rule 1 (4.0) + Rule 2 (2.5) + Rule 4 (2.0) = 8.5
	if r.Score != 8.5 {
		t.Fatalf("expected score 8.5, got %.1f", r.Score)
	}
	if r.Severity != "high" {
		t.Fatalf("expected severity high, got %s", r.Severity)
	}
	if r.Status != "fail" {
		t.Fatalf("expected status fail, got %s", r.Status)
	}

	hasDataExposure := false
	for _, reason := range r.Reasons {
		if reason.RuleID == "potential_data_exposure" {
			hasDataExposure = true
			break
		}
	}
	if !hasDataExposure {
		t.Fatal("expected potential_data_exposure rule to be triggered")
	}
}

func TestEvaluate_EntryPointAdded(t *testing.T) {
	d := delta.Delta{
		AddedNodes: []models.Node{
			{ID: "aws_lb.web", Type: "entry_point", ProviderType: "aws_lb", Attributes: map[string]any{"public": true}},
		},
		Summary: delta.DeltaSummary{AddedNodes: 1},
	}

	r := Evaluate(d)

	if r.Score != 3.0 {
		t.Fatalf("expected score 3.0, got %.1f", r.Score)
	}
	if r.Severity != "medium" {
		t.Fatalf("expected severity medium, got %s", r.Severity)
	}
	if r.Status != "warn" {
		t.Fatalf("expected status warn, got %s", r.Status)
	}
	if len(r.Reasons) != 1 {
		t.Fatalf("expected 1 reason, got %d", len(r.Reasons))
	}
	if r.Reasons[0].RuleID != "new_entry_point" {
		t.Fatalf("expected rule new_entry_point, got %s", r.Reasons[0].RuleID)
	}
}

func TestEvaluate_RemovalOnly(t *testing.T) {
	d := delta.Delta{
		RemovedNodes: []models.Node{
			{ID: "aws_instance.web", Type: "compute", ProviderType: "aws_instance", Attributes: map[string]any{"public": true}},
		},
		Summary: delta.DeltaSummary{RemovedNodes: 1},
	}

	r := Evaluate(d)

	if r.Score != 0.5 {
		t.Fatalf("expected score 0.5, got %.1f", r.Score)
	}
	if r.Severity != "low" {
		t.Fatalf("expected severity low, got %s", r.Severity)
	}
	if r.Status != "pass" {
		t.Fatalf("expected status pass, got %s", r.Status)
	}
	if len(r.Reasons) != 1 {
		t.Fatalf("expected 1 reason, got %d", len(r.Reasons))
	}
	if r.Reasons[0].RuleID != "resource_removed" {
		t.Fatalf("expected rule resource_removed, got %s", r.Reasons[0].RuleID)
	}
}

func TestEvaluate_NoChanges(t *testing.T) {
	d := delta.Delta{}

	r := Evaluate(d)

	if r.Score != 0.0 {
		t.Fatalf("expected score 0.0, got %.1f", r.Score)
	}
	if r.Severity != "low" {
		t.Fatalf("expected severity low, got %s", r.Severity)
	}
	if r.Status != "pass" {
		t.Fatalf("expected status pass, got %s", r.Status)
	}
	if len(r.Reasons) != 0 {
		t.Fatalf("expected 0 reasons, got %d", len(r.Reasons))
	}
}

func TestEvaluate_PublicAttrNonBool_DoesNotPanicOrTrigger(t *testing.T) {
	// Defensive: if `public` is somehow stored as a non-bool (e.g. JSON
	// round-trip producing string "true"), Rule 1 must not fire.
	d := delta.Delta{
		ChangedNodes: []delta.ChangedNode{
			{
				ID:           "aws_security_group.web",
				Type:         "access_control",
				ProviderType: "aws_security_group",
				ChangedAttributes: map[string]delta.ChangedAttribute{
					"public": {Before: "false", After: "true"},
				},
			},
		},
		Summary: delta.DeltaSummary{ChangedNodes: 1},
	}

	r := Evaluate(d)

	if r.Score != 0.0 {
		t.Fatalf("expected score 0.0 (rule should not fire on non-bool), got %.1f", r.Score)
	}
	for _, reason := range r.Reasons {
		if reason.RuleID == "public_exposure_introduced" {
			t.Fatal("public_exposure_introduced should not fire on non-bool attributes")
		}
	}
}

func TestEvaluate_RemovalCap(t *testing.T) {
	// 5 removed nodes -> only 2 reasons, total weight 1.0.
	removed := make([]models.Node, 5)
	for i := range removed {
		removed[i] = models.Node{
			ID:           fmt.Sprintf("aws_instance.web%d", i),
			Type:         "compute",
			ProviderType: "aws_instance",
			Attributes:   map[string]any{"public": false},
		}
	}
	d := delta.Delta{
		RemovedNodes: removed,
		Summary:      delta.DeltaSummary{RemovedNodes: 5},
	}

	r := Evaluate(d)

	if r.Score != 1.0 {
		t.Fatalf("expected score 1.0 (cap), got %.1f", r.Score)
	}
	if len(r.Reasons) != 2 {
		t.Fatalf("expected 2 reasons (cap), got %d", len(r.Reasons))
	}
}

// TestEvaluate_HighRiskFixture_NoRegression locks in the headline number
// ArchiteX has been quoting since v1.0: testdata/base -> testdata/head
// produces 9.0 / HIGH / fail (4.0 + 3.0 + 2.0). This is the score that
// landed on PR #1 in architex-test-customer during Phase 5 live validation
// and that the README, master.md, and llm.md cite verbatim.
//
// The Phase 6 (v1.1) "recognition only" PR must not change this number.
// If you intentionally rebalance rule weights or introduce a new always-on
// rule, update this test AND the cited numbers in the docs in the same PR.
func TestEvaluate_HighRiskFixture_NoRegression(t *testing.T) {
	d := delta.Delta{
		AddedNodes: []models.Node{
			{ID: "aws_lb.web", Type: "entry_point", ProviderType: "aws_lb", Attributes: map[string]any{"public": true}},
		},
		AddedEdges: []models.Edge{
			{From: "aws_lb.web", To: "aws_security_group.web", Type: "attached_to"},
			{From: "aws_lb.web", To: "aws_subnet.public", Type: "deployed_in"},
		},
		ChangedNodes: []delta.ChangedNode{
			{
				ID:           "aws_security_group.web",
				Type:         "access_control",
				ProviderType: "aws_security_group",
				ChangedAttributes: map[string]delta.ChangedAttribute{
					"public": {Before: false, After: true},
				},
			},
		},
		Summary: delta.DeltaSummary{AddedNodes: 1, AddedEdges: 2, ChangedNodes: 1},
	}

	r := Evaluate(d)

	if r.Score != 9.0 {
		t.Fatalf("regression: expected 9.0/10 on the canonical high-risk fixture, got %.1f", r.Score)
	}
	if r.Severity != "high" || r.Status != "fail" {
		t.Fatalf("regression: expected high/fail, got %s/%s", r.Severity, r.Status)
	}
	want := []string{"public_exposure_introduced", "new_entry_point", "potential_data_exposure"}
	if len(r.Reasons) != len(want) {
		t.Fatalf("expected %d reasons, got %d", len(want), len(r.Reasons))
	}
	for i, ruleID := range want {
		if r.Reasons[i].RuleID != ruleID {
			t.Errorf("reason[%d]: expected %q, got %q", i, ruleID, r.Reasons[i].RuleID)
		}
	}
}

// ---------------------------------------------------------------------------
// Phase 6 (v1.1) — AWS Top 10 rules
// ---------------------------------------------------------------------------

func TestEvaluate_S3PAB_Removed_TriggersBucketExposure(t *testing.T) {
	d := delta.Delta{
		RemovedNodes: []models.Node{
			{
				ID:           "aws_s3_bucket_public_access_block.logs",
				Type:         "access_control",
				ProviderType: "aws_s3_bucket_public_access_block",
				Attributes:   map[string]any{"public": false},
			},
		},
		Summary: delta.DeltaSummary{RemovedNodes: 1},
	}

	r := Evaluate(d)

	// 4.0 (s3_bucket_public_exposure) + 0.5 (resource_removed) = 4.5
	if r.Score != 4.5 {
		t.Fatalf("expected score 4.5, got %.1f", r.Score)
	}
	if !hasRuleID(r.Reasons, "s3_bucket_public_exposure") {
		t.Fatal("expected s3_bucket_public_exposure to fire on PAB removal")
	}
}

func TestEvaluate_S3Policy_Added_TriggersBucketExposure(t *testing.T) {
	d := delta.Delta{
		AddedNodes: []models.Node{
			{
				ID:           "aws_s3_bucket_policy.logs",
				Type:         "access_control",
				ProviderType: "aws_s3_bucket_policy",
				Attributes:   map[string]any{"public": false},
			},
		},
		Summary: delta.DeltaSummary{AddedNodes: 1},
	}

	r := Evaluate(d)

	if r.Score != 4.0 {
		t.Fatalf("expected score 4.0, got %.1f", r.Score)
	}
	if !hasRuleID(r.Reasons, "s3_bucket_public_exposure") {
		t.Fatal("expected s3_bucket_public_exposure to fire on bucket policy addition")
	}
}

func TestEvaluate_S3Exposure_CapAtTwoReasons(t *testing.T) {
	// Five PABs removed in one PR -> Phase 6 cap limits us to 2 reasons.
	removed := make([]models.Node, 5)
	for i := range removed {
		removed[i] = models.Node{
			ID:           fmt.Sprintf("aws_s3_bucket_public_access_block.logs%d", i),
			Type:         "access_control",
			ProviderType: "aws_s3_bucket_public_access_block",
			Attributes:   map[string]any{"public": false},
		}
	}
	d := delta.Delta{RemovedNodes: removed, Summary: delta.DeltaSummary{RemovedNodes: 5}}

	r := Evaluate(d)

	count := 0
	for _, reason := range r.Reasons {
		if reason.RuleID == "s3_bucket_public_exposure" {
			count++
		}
	}
	if count != 2 {
		t.Fatalf("expected s3_bucket_public_exposure cap=2, got %d", count)
	}
}

func TestEvaluate_IAMAdminAttachment_LiteralARN_Triggers(t *testing.T) {
	d := delta.Delta{
		AddedNodes: []models.Node{
			{
				ID:           "aws_iam_role_policy_attachment.app_admin",
				Type:         "identity",
				ProviderType: "aws_iam_role_policy_attachment",
				Attributes: map[string]any{
					"public":     false,
					"policy_arn": "arn:aws:iam::aws:policy/AdministratorAccess",
				},
			},
		},
		Summary: delta.DeltaSummary{AddedNodes: 1},
	}

	r := Evaluate(d)

	if r.Score != 3.5 {
		t.Fatalf("expected score 3.5, got %.1f", r.Score)
	}
	if !hasRuleID(r.Reasons, "iam_admin_policy_attached") {
		t.Fatal("expected iam_admin_policy_attached on AdministratorAccess attachment")
	}
}

func TestEvaluate_IAMAdminAttachment_NonAdminARN_DoesNotFire(t *testing.T) {
	d := delta.Delta{
		AddedNodes: []models.Node{
			{
				ID:           "aws_iam_role_policy_attachment.app_read",
				Type:         "identity",
				ProviderType: "aws_iam_role_policy_attachment",
				Attributes: map[string]any{
					"public":     false,
					"policy_arn": "arn:aws:iam::aws:policy/ReadOnlyAccess",
				},
			},
		},
		Summary: delta.DeltaSummary{AddedNodes: 1},
	}

	r := Evaluate(d)

	if hasRuleID(r.Reasons, "iam_admin_policy_attached") {
		t.Fatal("rule must not fire on benign managed policies")
	}
}

func TestEvaluate_IAMAdminAttachment_UnresolvedARN_DoesNotFire(t *testing.T) {
	// Variable-driven ARNs are captured by the parser as nil. We deliberately
	// do NOT guess at unresolved expressions -- documented in rules.go.
	d := delta.Delta{
		AddedNodes: []models.Node{
			{
				ID:           "aws_iam_role_policy_attachment.app",
				Type:         "identity",
				ProviderType: "aws_iam_role_policy_attachment",
				Attributes: map[string]any{
					"public":     false,
					"policy_arn": nil,
				},
			},
		},
		Summary: delta.DeltaSummary{AddedNodes: 1},
	}

	r := Evaluate(d)

	if hasRuleID(r.Reasons, "iam_admin_policy_attached") {
		t.Fatal("rule must not fire when policy_arn is unresolved")
	}
}

func TestEvaluate_LambdaFunctionURL_Triggers(t *testing.T) {
	d := delta.Delta{
		AddedNodes: []models.Node{
			{
				ID:           "aws_lambda_function_url.worker",
				Type:         "entry_point",
				ProviderType: "aws_lambda_function_url",
				Attributes:   map[string]any{"public": true},
			},
		},
		Summary: delta.DeltaSummary{AddedNodes: 1},
	}

	r := Evaluate(d)

	// new_entry_point (3.0) + lambda_public_url_introduced (3.0) = 6.0
	if r.Score != 6.0 {
		t.Fatalf("expected score 6.0, got %.1f", r.Score)
	}
	if !hasRuleID(r.Reasons, "lambda_public_url_introduced") {
		t.Fatal("expected lambda_public_url_introduced on Lambda URL addition")
	}
	if !hasRuleID(r.Reasons, "new_entry_point") {
		t.Fatal("expected the generic new_entry_point rule to ALSO fire (additive design)")
	}
}

// TestEvaluate_Phase6Integration_Top10Fixture is the headline integration
// test for v1.1. It feeds an in-memory delta that mirrors the
// testdata/top10_base -> testdata/top10_head scenario:
//
//   - PAB removed                                   -> s3_bucket_public_exposure (4.0)
//   - aws_iam_role_policy_attachment.app_admin added with AdministratorAccess
//     -> iam_admin_policy_attached (3.5)
//   - aws_lambda_function_url added                 -> lambda_public_url_introduced (3.0)
//     -> new_entry_point (3.0)
//   - resource_removed (PAB)                        -> 0.5
//
// Total weight: 14.0, capped at 10.0 -> high / fail.
func TestEvaluate_Phase6Integration_Top10Fixture(t *testing.T) {
	d := delta.Delta{
		AddedNodes: []models.Node{
			{
				ID:           "aws_iam_role_policy_attachment.app_admin",
				Type:         "identity",
				ProviderType: "aws_iam_role_policy_attachment",
				Attributes: map[string]any{
					"public":     false,
					"policy_arn": "arn:aws:iam::aws:policy/AdministratorAccess",
				},
			},
			{
				ID:           "aws_lambda_function_url.worker",
				Type:         "entry_point",
				ProviderType: "aws_lambda_function_url",
				Attributes:   map[string]any{"public": true},
			},
		},
		RemovedNodes: []models.Node{
			{
				ID:           "aws_s3_bucket_public_access_block.logs",
				Type:         "access_control",
				ProviderType: "aws_s3_bucket_public_access_block",
				Attributes:   map[string]any{"public": false},
			},
		},
		Summary: delta.DeltaSummary{AddedNodes: 2, RemovedNodes: 1},
	}

	r := Evaluate(d)

	if r.Score != 10.0 {
		t.Fatalf("expected capped score 10.0, got %.1f", r.Score)
	}
	if r.Severity != "high" || r.Status != "fail" {
		t.Fatalf("expected high/fail, got %s/%s", r.Severity, r.Status)
	}

	wantRules := []string{
		"s3_bucket_public_exposure",
		"iam_admin_policy_attached",
		"lambda_public_url_introduced",
		"new_entry_point",
		"resource_removed",
	}
	for _, want := range wantRules {
		if !hasRuleID(r.Reasons, want) {
			t.Errorf("expected rule %q to fire on the top10 integration fixture", want)
		}
	}
}

func hasRuleID(reasons []RiskReason, ruleID string) bool {
	for _, r := range reasons {
		if r.RuleID == ruleID {
			return true
		}
	}
	return false
}

func TestEvaluate_ScoreCap(t *testing.T) {
	d := delta.Delta{
		AddedNodes: []models.Node{
			{ID: "aws_db_instance.main", Type: "data", ProviderType: "aws_db_instance", Attributes: map[string]any{"public": false}},
			{ID: "aws_lb.web", Type: "entry_point", ProviderType: "aws_lb", Attributes: map[string]any{"public": true}},
		},
		ChangedNodes: []delta.ChangedNode{
			{
				ID:           "aws_security_group.web",
				Type:         "access_control",
				ProviderType: "aws_security_group",
				ChangedAttributes: map[string]delta.ChangedAttribute{
					"public": {Before: false, After: true},
				},
			},
		},
		Summary: delta.DeltaSummary{AddedNodes: 2, ChangedNodes: 1},
	}

	r := Evaluate(d)

	// Rule 1 (4.0) + Rule 2 (2.5) + Rule 3 (3.0) + Rule 4 (2.0) = 11.5, capped at 10.0
	if r.Score != 10.0 {
		t.Fatalf("expected score 10.0, got %.1f", r.Score)
	}
	if r.Severity != "high" {
		t.Fatalf("expected severity high, got %s", r.Severity)
	}
	if r.Status != "fail" {
		t.Fatalf("expected status fail, got %s", r.Status)
	}
}
