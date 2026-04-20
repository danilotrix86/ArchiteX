package baseline

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"architex/models"
)

func TestFromGraph_DedupesSortsAndDropsEmpty(t *testing.T) {
	g := models.Graph{
		Nodes: []models.Node{
			{ID: "aws_lb.web", ProviderType: "aws_lb", Type: "entry_point"},
			{ID: "aws_lb.api", ProviderType: "aws_lb", Type: "entry_point"},
			{ID: "aws_vpc.main", ProviderType: "aws_vpc", Type: "network"},
			{ID: "aws_db_instance.users", ProviderType: "aws_db_instance", Type: "data"},
			{ID: "broken.x", ProviderType: "", Type: ""},
		},
		Edges: []models.Edge{
			{From: "aws_lb.web", To: "aws_vpc.main", Type: "deployed_in"},
			{From: "aws_lb.api", To: "aws_vpc.main", Type: "deployed_in"},
			{From: "aws_db_instance.users", To: "aws_vpc.main", Type: "deployed_in"},
			{From: "ghost", To: "aws_vpc.main", Type: "x"},
		},
	}
	b := FromGraph(g, time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC))

	if got, want := b.SchemaVersion, SchemaVersion; got != want {
		t.Errorf("schema_version = %q, want %q", got, want)
	}
	if !b.GeneratedAt.Equal(time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)) {
		t.Errorf("generated_at = %v, want UTC literal", b.GeneratedAt)
	}
	if got, want := b.ProviderTypes, []string{"aws_db_instance", "aws_lb", "aws_vpc"}; !reflect.DeepEqual(got, want) {
		t.Errorf("provider_types = %v, want %v", got, want)
	}
	if got, want := b.AbstractTypes, []string{"data", "entry_point", "network"}; !reflect.DeepEqual(got, want) {
		t.Errorf("abstract_types = %v, want %v", got, want)
	}
	if got, want := b.EdgePairs, []string{"aws_db_instance|aws_vpc", "aws_lb|aws_vpc"}; !reflect.DeepEqual(got, want) {
		t.Errorf("edge_pairs = %v, want %v", got, want)
	}
}

func TestFromGraph_EmptyGraph(t *testing.T) {
	b := FromGraph(models.Graph{}, time.Now())
	if b.SchemaVersion != SchemaVersion {
		t.Errorf("schema_version = %q", b.SchemaVersion)
	}
	if len(b.ProviderTypes) != 0 || len(b.AbstractTypes) != 0 || len(b.EdgePairs) != 0 {
		t.Errorf("empty graph must produce empty sets, got %+v", b)
	}
}

func TestSaveLoad_Roundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".architex", "baseline.json")

	in := &Baseline{
		SchemaVersion: SchemaVersion,
		GeneratedAt:   time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC),
		ProviderTypes: []string{"aws_lb", "aws_vpc"},
		AbstractTypes: []string{"entry_point", "network"},
		EdgePairs:     []string{"aws_lb|aws_vpc"},
	}
	if err := Save(path, in); err != nil {
		t.Fatalf("Save: %v", err)
	}
	out, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(in, out) {
		t.Errorf("roundtrip mismatch:\n in=%#v\nout=%#v", in, out)
	}
}

func TestLoad_AbsentFileReturnsNilNil(t *testing.T) {
	dir := t.TempDir()
	b, err := Load(filepath.Join(dir, "nope", "baseline.json"))
	if err != nil {
		t.Fatalf("absent file must not error, got %v", err)
	}
	if b != nil {
		t.Errorf("absent file must return nil baseline, got %#v", b)
	}
}

func TestLoad_BadSchemaVersionErrors(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "baseline.json")
	raw, _ := json.Marshal(map[string]any{
		"schema_version": "999",
		"provider_types": []string{"aws_lb"},
	})
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := Load(path)
	if err == nil {
		t.Fatalf("expected schema-version error, got nil")
	}
}

func TestLoad_NormalizesUnsortedAndDuped(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "baseline.json")
	raw, _ := json.Marshal(map[string]any{
		"schema_version": SchemaVersion,
		"generated_at":   "2026-04-19T12:00:00Z",
		"provider_types": []string{"aws_vpc", "aws_lb", "aws_lb"},
		"abstract_types": []string{"network", "entry_point", " entry_point "},
		"edge_pairs":     []string{"aws_lb|aws_vpc"},
	})
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	b, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got, want := b.ProviderTypes, []string{"aws_lb", "aws_vpc"}; !reflect.DeepEqual(got, want) {
		t.Errorf("provider_types not normalized: %v", got)
	}
	if got, want := b.AbstractTypes, []string{"entry_point", "network"}; !reflect.DeepEqual(got, want) {
		t.Errorf("abstract_types not normalized: %v", got)
	}
}

func TestMerge_UnionAndDedup(t *testing.T) {
	a := &Baseline{
		SchemaVersion: SchemaVersion,
		ProviderTypes: []string{"aws_lb", "aws_vpc"},
		AbstractTypes: []string{"entry_point", "network"},
		EdgePairs:     []string{"aws_lb|aws_vpc"},
	}
	b := &Baseline{
		SchemaVersion: SchemaVersion,
		ProviderTypes: []string{"aws_vpc", "aws_kms_key"},
		AbstractTypes: []string{"network", "identity"},
		EdgePairs:     []string{"aws_kms_alias|aws_kms_key", "aws_lb|aws_vpc"},
	}
	m := Merge(a, b)
	if got, want := m.ProviderTypes, []string{"aws_kms_key", "aws_lb", "aws_vpc"}; !reflect.DeepEqual(got, want) {
		t.Errorf("provider merge = %v", got)
	}
	if got, want := m.AbstractTypes, []string{"entry_point", "identity", "network"}; !reflect.DeepEqual(got, want) {
		t.Errorf("abstract merge = %v", got)
	}
	if got, want := m.EdgePairs, []string{"aws_kms_alias|aws_kms_key", "aws_lb|aws_vpc"}; !reflect.DeepEqual(got, want) {
		t.Errorf("edge merge = %v", got)
	}
}

func TestMerge_NilHandling(t *testing.T) {
	if got := Merge(nil, nil); got != nil {
		t.Errorf("nil/nil = %v, want nil", got)
	}

	a := &Baseline{SchemaVersion: SchemaVersion, ProviderTypes: []string{"aws_lb"}}
	if got := Merge(nil, a); !reflect.DeepEqual(got.ProviderTypes, a.ProviderTypes) {
		t.Errorf("merge(nil,a) lost data: %v", got)
	}
	if got := Merge(a, nil); !reflect.DeepEqual(got.ProviderTypes, a.ProviderTypes) {
		t.Errorf("merge(a,nil) lost data: %v", got)
	}
}

func TestHasMethods_NilReceiverDisablesRules(t *testing.T) {
	var b *Baseline
	if b.HasProviderType("aws_lb") {
		t.Errorf("nil baseline must report HasProviderType=false (rules silenced)")
	}
	if b.HasAbstractType("entry_point") {
		t.Errorf("nil baseline must report HasAbstractType=false")
	}
	if b.HasEdgePair("aws_lb", "aws_vpc") {
		t.Errorf("nil baseline must report HasEdgePair=false")
	}
}

func TestHasMethods_PopulatedBaseline(t *testing.T) {
	b := &Baseline{
		SchemaVersion: SchemaVersion,
		ProviderTypes: []string{"aws_lb", "aws_vpc"},
		AbstractTypes: []string{"entry_point", "network"},
		EdgePairs:     []string{"aws_lb|aws_vpc"},
	}
	if !b.HasProviderType("aws_lb") || b.HasProviderType("aws_kms_key") {
		t.Errorf("HasProviderType lookups wrong")
	}
	if !b.HasAbstractType("entry_point") || b.HasAbstractType("identity") {
		t.Errorf("HasAbstractType lookups wrong")
	}
	if !b.HasEdgePair("aws_lb", "aws_vpc") || b.HasEdgePair("aws_lb", "aws_kms_key") {
		t.Errorf("HasEdgePair lookups wrong")
	}
}

func TestSave_AtomicReplace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "baseline.json")

	first := &Baseline{SchemaVersion: SchemaVersion, ProviderTypes: []string{"aws_lb"}}
	if err := Save(path, first); err != nil {
		t.Fatalf("save 1: %v", err)
	}
	second := &Baseline{SchemaVersion: SchemaVersion, ProviderTypes: []string{"aws_lb", "aws_vpc"}}
	if err := Save(path, second); err != nil {
		t.Fatalf("save 2: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(got.ProviderTypes, []string{"aws_lb", "aws_vpc"}) {
		t.Errorf("expected second snapshot to win, got %v", got.ProviderTypes)
	}

	matches, _ := filepath.Glob(filepath.Join(dir, ".architex-baseline-*.tmp"))
	if len(matches) != 0 {
		t.Errorf("Save left tempfiles behind: %v", matches)
	}
}
