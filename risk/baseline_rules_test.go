package risk

import (
	"strings"
	"testing"
	"time"

	"architex/baseline"
	"architex/config"
	"architex/delta"
	"architex/models"
	rulesbaseline "architex/risk/rules/baseline"
)

func newTestBaseline(provider, abstract, edges []string) *baseline.Baseline {
	return &baseline.Baseline{
		SchemaVersion: baseline.SchemaVersion,
		ProviderTypes: provider,
		AbstractTypes: abstract,
		EdgePairs:     edges,
	}
}

func TestEvaluateWithBaseline_NilBaseline_NoFirstTimeReasons(t *testing.T) {
	d := delta.Delta{
		AddedNodes: []models.Node{
			{ID: "aws_kms_key.k", ProviderType: "aws_kms_key", Type: "identity"},
		},
		AddedEdges: []models.Edge{
			{From: "aws_kms_alias.a", To: "aws_kms_key.k", Type: "applies_to"},
		},
	}
	r := EvaluateWithBaseline(d, nil, nil, time.Time{})
	for _, reason := range r.Reasons {
		if strings.HasPrefix(reason.RuleID, "first_time_") {
			t.Errorf("first_time_* fired with nil baseline: %s", reason.RuleID)
		}
	}
}

func TestFirstTimeResourceType_FiresOnce_DedupAndCap(t *testing.T) {
	bl := newTestBaseline(
		[]string{"aws_lb", "aws_vpc"},
		[]string{"entry_point", "network"},
		[]string{"aws_lb|aws_vpc"},
	)

	d := delta.Delta{
		AddedNodes: []models.Node{
			// aws_kms_key x3 should still fire only once.
			{ID: "aws_kms_key.a", ProviderType: "aws_kms_key", Type: "identity"},
			{ID: "aws_kms_key.b", ProviderType: "aws_kms_key", Type: "identity"},
			{ID: "aws_kms_key.c", ProviderType: "aws_kms_key", Type: "identity"},
			// aws_sns_topic - second novel type, also fires once.
			{ID: "aws_sns_topic.alerts", ProviderType: "aws_sns_topic", Type: "data"},
			// aws_lb is in baseline; must NOT fire.
			{ID: "aws_lb.web", ProviderType: "aws_lb", Type: "entry_point"},
		},
	}
	got := rulesbaseline.EvaluateFirstTimeResourceType(d, bl)
	if len(got) != 2 {
		t.Fatalf("expected exactly 2 first_time_resource_type reasons (one per novel type, capped), got %d: %+v", len(got), got)
	}
	seen := map[string]bool{}
	for _, r := range got {
		if r.RuleID != "first_time_resource_type" {
			t.Errorf("wrong rule id: %s", r.RuleID)
		}
		if r.Weight != 1.0 {
			t.Errorf("weight = %v, want 1.0", r.Weight)
		}
		if r.ResourceID == "" {
			t.Errorf("ResourceID must be populated for suppression matching")
		}
		seen[r.Message] = true
	}
	if len(seen) != 2 {
		t.Errorf("messages must be distinct (one per type), got %v", seen)
	}
}

func TestFirstTimeResourceType_CapAtTwo(t *testing.T) {
	bl := newTestBaseline(nil, nil, nil)
	d := delta.Delta{
		AddedNodes: []models.Node{
			{ID: "aws_lb.a", ProviderType: "aws_lb", Type: "entry_point"},
			{ID: "aws_kms_key.k", ProviderType: "aws_kms_key", Type: "identity"},
			{ID: "aws_sns_topic.t", ProviderType: "aws_sns_topic", Type: "data"},
		},
	}
	got := rulesbaseline.EvaluateFirstTimeResourceType(d, bl)
	if len(got) != rulesbaseline.CapPerRule {
		t.Errorf("expected cap of %d reasons, got %d", rulesbaseline.CapPerRule, len(got))
	}
}

func TestFirstTimeAbstractType_FiresOncePerCategory(t *testing.T) {
	bl := newTestBaseline(
		[]string{"aws_vpc"},
		[]string{"network"},
		nil,
	)
	d := delta.Delta{
		AddedNodes: []models.Node{
			{ID: "aws_lb.web", ProviderType: "aws_lb", Type: "entry_point"},
			{ID: "aws_apigatewayv2_api.http", ProviderType: "aws_apigatewayv2_api", Type: "entry_point"},
			{ID: "aws_db_instance.users", ProviderType: "aws_db_instance", Type: "data"},
			{ID: "aws_subnet.main", ProviderType: "aws_subnet", Type: "network"},
		},
	}
	got := rulesbaseline.EvaluateFirstTimeAbstractType(d, bl)
	if len(got) != 2 {
		t.Fatalf("expected 2 abstract reasons (entry_point + data), got %d: %+v", len(got), got)
	}
	for _, r := range got {
		if r.RuleID != "first_time_abstract_type" {
			t.Errorf("wrong rule id: %s", r.RuleID)
		}
		if r.Weight != 1.5 {
			t.Errorf("weight = %v, want 1.5", r.Weight)
		}
	}
}

func TestFirstTimeEdgePair_RequiresKnownEndpoints(t *testing.T) {
	bl := newTestBaseline(nil, nil, []string{"aws_lb|aws_vpc"})

	d := delta.Delta{
		AddedNodes: []models.Node{
			{ID: "aws_kms_alias.a", ProviderType: "aws_kms_alias"},
			{ID: "aws_kms_key.k", ProviderType: "aws_kms_key"},
			{ID: "aws_lb.web", ProviderType: "aws_lb"},
			{ID: "aws_vpc.main", ProviderType: "aws_vpc"},
		},
		AddedEdges: []models.Edge{
			{From: "aws_kms_alias.a", To: "aws_kms_key.k", Type: "applies_to"},
			// duplicate pair -- should NOT double-fire
			{From: "aws_kms_alias.b", To: "aws_kms_key.k2", Type: "applies_to"},
			{From: "aws_lb.web", To: "aws_vpc.main", Type: "deployed_in"}, // known
			{From: "ghost.x", To: "aws_vpc.main", Type: "x"},              // unknown endpoint -> skip
		},
	}
	// Note: aws_kms_alias.b / aws_kms_key.k2 are not in the node table; the
	// rule should silently skip the second edge because it cannot resolve
	// the endpoint provider types. So only the first kms edge fires.
	got := rulesbaseline.EvaluateFirstTimeEdgePair(d, bl)
	if len(got) != 1 {
		t.Fatalf("expected 1 first_time_edge_pair reason, got %d: %+v", len(got), got)
	}
	if got[0].Weight != 0.5 {
		t.Errorf("weight = %v, want 0.5", got[0].Weight)
	}
	if !strings.Contains(got[0].Message, "aws_kms_alias -> aws_kms_key") {
		t.Errorf("message wrong: %q", got[0].Message)
	}
}

func TestFirstTimeEdgePair_DedupesSamePairAcrossInstances(t *testing.T) {
	bl := newTestBaseline(nil, nil, nil)
	d := delta.Delta{
		AddedNodes: []models.Node{
			{ID: "aws_lb.a", ProviderType: "aws_lb"},
			{ID: "aws_lb.b", ProviderType: "aws_lb"},
			{ID: "aws_vpc.main", ProviderType: "aws_vpc"},
		},
		AddedEdges: []models.Edge{
			{From: "aws_lb.a", To: "aws_vpc.main", Type: "deployed_in"},
			{From: "aws_lb.b", To: "aws_vpc.main", Type: "deployed_in"},
		},
	}
	got := rulesbaseline.EvaluateFirstTimeEdgePair(d, bl)
	if len(got) != 1 {
		t.Errorf("same pair must dedupe to 1 reason, got %d", len(got))
	}
}

func TestEvaluateWithBaseline_StackingWithExistingRules(t *testing.T) {
	bl := newTestBaseline(
		[]string{"aws_vpc"},
		[]string{"network"},
		nil,
	)
	d := delta.Delta{
		AddedNodes: []models.Node{
			{
				ID: "aws_lb.web", ProviderType: "aws_lb", Type: "entry_point",
				Attributes: map[string]any{"public": true},
			},
		},
	}
	r := EvaluateWithBaseline(d, nil, bl, time.Time{})

	want := map[string]bool{
		"new_entry_point":          false,
		"first_time_resource_type": false,
		"first_time_abstract_type": false,
	}
	for _, reason := range r.Reasons {
		if _, ok := want[reason.RuleID]; ok {
			want[reason.RuleID] = true
		}
	}
	for k, fired := range want {
		if !fired {
			t.Errorf("expected rule %s to fire, but it did not. reasons=%+v", k, r.Reasons)
		}
	}

	// Score should sum to 3.0 (new_entry_point) + 1.0 + 1.5 = 5.5.
	if r.Score != 5.5 {
		t.Errorf("expected stacked score 5.5, got %v (reasons=%+v)", r.Score, r.Reasons)
	}
}

func TestEvaluateWithBaseline_RespectsConfigSuppression(t *testing.T) {
	bl := newTestBaseline(
		[]string{"aws_vpc"},
		[]string{"network"},
		nil,
	)
	d := delta.Delta{
		AddedNodes: []models.Node{
			{ID: "aws_kms_key.k", ProviderType: "aws_kms_key", Type: "identity"},
		},
	}

	// Build a config that disables first_time_abstract_type and suppresses
	// the resource-type firing on aws_kms_key.k specifically. After that,
	// no first_time_* reasons should remain.
	disabled := false
	cfg := &config.Config{
		Rules: map[string]config.RuleConfig{
			"first_time_abstract_type": {Enabled: &disabled},
		},
		Suppressions: []config.Suppression{
			{
				Rule:     "first_time_resource_type",
				Resource: "aws_kms_key.k",
				Reason:   "team owns this",
				Source:   "config:.architex.yml",
			},
		},
	}

	r := EvaluateWithBaseline(d, cfg, bl, time.Now())

	for _, reason := range r.Reasons {
		if strings.HasPrefix(reason.RuleID, "first_time_") {
			t.Errorf("first_time_* leaked past config: %+v", reason)
		}
	}
	if len(r.Suppressed) != 1 || r.Suppressed[0].RuleID != "first_time_resource_type" {
		t.Errorf("expected first_time_resource_type to land in Suppressed, got %+v", r.Suppressed)
	}
}
