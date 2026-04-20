package risk

import (
	"testing"
	"time"

	"architex/config"
	"architex/delta"
	"architex/models"
)

// EvaluateWith with cfg=nil must produce bit-identical results to v1.1's
// Evaluate. This is the backward-compat guarantee for zero-config repos.
func TestEvaluateWith_NilConfig_BehavesAsV11(t *testing.T) {
	d := delta.Delta{
		ChangedNodes: []delta.ChangedNode{
			{
				ID:                "aws_security_group.web",
				Type:              "access_control",
				ProviderType:      "aws_security_group",
				ChangedAttributes: map[string]delta.ChangedAttribute{"public": {Before: false, After: true}},
			},
		},
		Summary: delta.DeltaSummary{ChangedNodes: 1},
	}

	a := Evaluate(d)
	b := EvaluateWith(d, nil, time.Now())

	if a.Score != b.Score || a.Severity != b.Severity || a.Status != b.Status {
		t.Fatalf("nil-cfg Evaluate diverged from default Evaluate:\n a=%+v\n b=%+v", a, b)
	}
	if len(a.Reasons) != len(b.Reasons) {
		t.Fatalf("reason count mismatch: a=%d b=%d", len(a.Reasons), len(b.Reasons))
	}
	if len(b.Suppressed) != 0 {
		t.Fatalf("nil cfg must produce no suppressions, got %d", len(b.Suppressed))
	}
}

func TestEvaluateWith_RuleDisabled(t *testing.T) {
	d := delta.Delta{
		AddedNodes: []models.Node{
			{ID: "aws_db_instance.main", Type: "data", ProviderType: "aws_db_instance",
				Attributes: map[string]any{"public": false}},
		},
		Summary: delta.DeltaSummary{AddedNodes: 1},
	}
	disabled := false
	cfg := &config.Config{
		Rules: map[string]config.RuleConfig{
			"new_data_resource": {Enabled: &disabled},
		},
	}
	r := EvaluateWith(d, cfg, time.Now())
	if r.Score != 0 {
		t.Fatalf("expected score 0 (rule disabled), got %.1f", r.Score)
	}
	for _, reason := range r.Reasons {
		if reason.RuleID == "new_data_resource" {
			t.Fatalf("disabled rule must not appear in reasons: %+v", reason)
		}
	}
}

func TestEvaluateWith_WeightOverride(t *testing.T) {
	d := delta.Delta{
		AddedNodes: []models.Node{
			{ID: "aws_db_instance.main", Type: "data", ProviderType: "aws_db_instance",
				Attributes: map[string]any{"public": false}},
		},
		Summary: delta.DeltaSummary{AddedNodes: 1},
	}
	w := 8.0
	cfg := &config.Config{
		Rules: map[string]config.RuleConfig{
			"new_data_resource": {Weight: &w},
		},
	}
	r := EvaluateWith(d, cfg, time.Now())
	if r.Score != 8.0 {
		t.Fatalf("expected score 8.0 (weight override), got %.1f", r.Score)
	}
	if r.Severity != "high" {
		t.Fatalf("expected severity high, got %s", r.Severity)
	}
}

func TestEvaluateWith_SuppressionMovesFinding(t *testing.T) {
	d := delta.Delta{
		AddedNodes: []models.Node{
			{ID: "aws_db_instance.main", Type: "data", ProviderType: "aws_db_instance",
				Attributes: map[string]any{"public": false}},
		},
		Summary: delta.DeltaSummary{AddedNodes: 1},
	}
	cfg := &config.Config{
		Suppressions: []config.Suppression{
			{
				Rule:     "new_data_resource",
				Resource: "aws_db_instance.main",
				Reason:   "intentionally tracking this DB",
				Source:   "config:.architex.yml",
			},
		},
	}
	r := EvaluateWith(d, cfg, time.Now())
	if r.Score != 0 {
		t.Fatalf("expected score 0 (finding suppressed), got %.1f", r.Score)
	}
	if len(r.Suppressed) != 1 {
		t.Fatalf("expected 1 suppressed finding, got %d", len(r.Suppressed))
	}
	if r.Suppressed[0].RuleID != "new_data_resource" {
		t.Fatalf("unexpected suppressed rule: %s", r.Suppressed[0].RuleID)
	}
	if r.Suppressed[0].ResourceID != "aws_db_instance.main" {
		t.Fatalf("unexpected suppressed resource: %s", r.Suppressed[0].ResourceID)
	}
	if r.Suppressed[0].Reason == "" {
		t.Fatal("suppressed reason must be carried over")
	}
}

func TestEvaluateWith_ExpiredSuppression_StillDropsButFlagged(t *testing.T) {
	d := delta.Delta{
		AddedNodes: []models.Node{
			{ID: "aws_db_instance.main", Type: "data", ProviderType: "aws_db_instance",
				Attributes: map[string]any{"public": false}},
		},
		Summary: delta.DeltaSummary{AddedNodes: 1},
	}
	cfg := &config.Config{
		Suppressions: []config.Suppression{
			{
				Rule:     "new_data_resource",
				Resource: "aws_db_instance.main",
				Reason:   "stale",
				Expires:  "2020-01-01",
			},
		},
	}
	r := EvaluateWith(d, cfg, time.Now())
	if r.Score != 0 {
		t.Fatalf("expired suppressions still drop the rule, score should be 0; got %.1f", r.Score)
	}
	if len(r.Suppressed) != 1 || !r.Suppressed[0].Expired {
		t.Fatalf("expected expired suppression to be flagged, got %+v", r.Suppressed)
	}
}

func TestEvaluateWith_CrossResourceReason_NotSuppressed(t *testing.T) {
	d := delta.Delta{
		AddedNodes: []models.Node{
			{ID: "aws_db_instance.main", Type: "data", ProviderType: "aws_db_instance",
				Attributes: map[string]any{"public": false}},
		},
		ChangedNodes: []delta.ChangedNode{
			{
				ID: "aws_security_group.web", Type: "access_control",
				ProviderType:      "aws_security_group",
				ChangedAttributes: map[string]delta.ChangedAttribute{"public": {Before: false, After: true}},
			},
		},
		Summary: delta.DeltaSummary{AddedNodes: 1, ChangedNodes: 1},
	}
	// potential_data_exposure is cross-resource (no ResourceID); a (rule,
	// resource) suppression must NOT silence it.
	cfg := &config.Config{
		Suppressions: []config.Suppression{
			{
				Rule:     "potential_data_exposure",
				Resource: "aws_db_instance.main",
				Reason:   "should not match",
			},
		},
	}
	r := EvaluateWith(d, cfg, time.Now())
	found := false
	for _, reason := range r.Reasons {
		if reason.RuleID == "potential_data_exposure" {
			found = true
		}
	}
	if !found {
		t.Fatal("cross-resource rule must not be suppressed by (rule, resource) tuple")
	}
}

func TestEvaluateWith_ThresholdOverride(t *testing.T) {
	d := delta.Delta{
		AddedNodes: []models.Node{
			{ID: "aws_db_instance.main", Type: "data", ProviderType: "aws_db_instance",
				Attributes: map[string]any{"public": false}},
		},
		Summary: delta.DeltaSummary{AddedNodes: 1},
	}
	warn := 1.0
	fail := 2.0
	cfg := &config.Config{
		Thresholds: config.Thresholds{Warn: &warn, Fail: &fail},
	}
	r := EvaluateWith(d, cfg, time.Now())
	if r.Score != 2.5 {
		t.Fatalf("expected score 2.5, got %.1f", r.Score)
	}
	if r.Severity != "high" {
		t.Fatalf("expected severity high under fail=2.0, got %s", r.Severity)
	}
}
