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
