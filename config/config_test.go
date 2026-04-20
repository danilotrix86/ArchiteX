package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeTempConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestLoad_AbsentFile_ReturnsNilNoError(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "nope.yml"))
	if err != nil {
		t.Fatalf("expected nil err for absent file, got %v", err)
	}
	if cfg != nil {
		t.Fatalf("expected nil cfg, got %#v", cfg)
	}
}

func TestLoad_FullDocument(t *testing.T) {
	body := `
rules:
  s3_bucket_public_exposure:
    weight: 5.5
  iam_admin_policy_attached:
    enabled: false
thresholds:
  warn: 2.0
  fail: 8.0
ignore:
  paths:
    - "**/legacy/**"
suppressions:
  - rule: lambda_public_url_introduced
    resource: aws_lambda_function_url.maintenance
    reason: "Maintenance window only"
    expires: "2030-01-01"
`
	cfg, err := Load(writeTempConfig(t, body))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.WarnThreshold() != 2.0 {
		t.Fatalf("warn threshold = %v, want 2.0", cfg.WarnThreshold())
	}
	if cfg.FailThreshold() != 8.0 {
		t.Fatalf("fail threshold = %v, want 8.0", cfg.FailThreshold())
	}
	if cfg.RuleEnabled("iam_admin_policy_attached") {
		t.Fatal("iam_admin_policy_attached should be disabled")
	}
	if !cfg.RuleEnabled("any_other_rule") {
		t.Fatal("unconfigured rules should default to enabled")
	}
	if w := cfg.RuleWeight("s3_bucket_public_exposure", 4.0); w != 5.5 {
		t.Fatalf("weight = %v, want 5.5", w)
	}
	if w := cfg.RuleWeight("unknown_rule", 1.25); w != 1.25 {
		t.Fatalf("fallback weight = %v, want 1.25", w)
	}
	if !cfg.IsPathIgnored("infra/legacy/main.tf") {
		t.Fatal("legacy path should be ignored")
	}
	if cfg.IsPathIgnored("infra/main.tf") {
		t.Fatal("non-legacy path should not be ignored")
	}

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	sup, expired, ok := cfg.MatchSuppression(
		"lambda_public_url_introduced",
		"aws_lambda_function_url.maintenance",
		now,
	)
	if !ok || expired {
		t.Fatalf("expected active suppression, got ok=%v expired=%v", ok, expired)
	}
	if sup.Reason == "" {
		t.Fatal("reason must be propagated")
	}
}

func TestValidate_RejectsInvertedThresholds(t *testing.T) {
	body := `
thresholds:
  warn: 9.0
  fail: 5.0
`
	if _, err := Load(writeTempConfig(t, body)); err == nil {
		t.Fatal("expected error for warn > fail")
	}
}

func TestValidate_RequiresReason(t *testing.T) {
	body := `
suppressions:
  - rule: foo
    resource: bar.baz
`
	if _, err := Load(writeTempConfig(t, body)); err == nil {
		t.Fatal("expected error for missing reason")
	}
}

func TestMatchSuppression_ResourceWildcard(t *testing.T) {
	cfg := &Config{
		Suppressions: []Suppression{
			{Rule: "s3_bucket_public_exposure", Resource: "aws_s3_bucket.legacy_*", Reason: "test"},
		},
	}
	if _, _, ok := cfg.MatchSuppression(
		"s3_bucket_public_exposure",
		"aws_s3_bucket.legacy_assets",
		time.Now(),
	); !ok {
		t.Fatal("wildcard suppression should match")
	}
	if _, _, ok := cfg.MatchSuppression(
		"s3_bucket_public_exposure",
		"aws_s3_bucket.modern_assets",
		time.Now(),
	); ok {
		t.Fatal("wildcard must not match unrelated resource")
	}
}

func TestMatchSuppression_Expired(t *testing.T) {
	cfg := &Config{
		Suppressions: []Suppression{
			{
				Rule:     "iam_admin_policy_attached",
				Resource: "aws_iam_role_policy_attachment.admin",
				Reason:   "tracked in JIRA-123",
				Expires:  "2020-01-01",
			},
		},
	}
	_, expired, ok := cfg.MatchSuppression(
		"iam_admin_policy_attached",
		"aws_iam_role_policy_attachment.admin",
		time.Now(),
	)
	if !ok {
		t.Fatal("expired suppression should still match (so the rule is dropped)")
	}
	if !expired {
		t.Fatal("expected expired = true")
	}
}

func TestNilConfig_BehavesAsDefault(t *testing.T) {
	var cfg *Config
	if cfg.WarnThreshold() != DefaultThresholdWarn {
		t.Fatal("nil cfg must yield default warn threshold")
	}
	if cfg.FailThreshold() != DefaultThresholdFail {
		t.Fatal("nil cfg must yield default fail threshold")
	}
	if !cfg.RuleEnabled("anything") {
		t.Fatal("nil cfg must report all rules enabled")
	}
	if w := cfg.RuleWeight("anything", 3.5); w != 3.5 {
		t.Fatalf("nil cfg must echo fallback weight, got %v", w)
	}
	if cfg.IsPathIgnored("anything") {
		t.Fatal("nil cfg must ignore nothing")
	}
	if _, _, ok := cfg.MatchSuppression("a", "b", time.Now()); ok {
		t.Fatal("nil cfg must match no suppressions")
	}
}

func TestGlobMatch_DoubleStar(t *testing.T) {
	cases := []struct {
		pattern, name string
		want          bool
	}{
		{"**/legacy/**", "infra/legacy/main.tf", true},
		{"**/legacy/**", "infra/legacy/sub/x.tf", true},
		{"**/legacy/**", "infra/main.tf", false},
		{"*.tf", "main.tf", true},
		{"*.tf", "sub/main.tf", false},
		{"infra/*.tf", "infra/main.tf", true},
	}
	for _, tc := range cases {
		got, err := globMatch(tc.pattern, tc.name)
		if err != nil {
			t.Fatalf("globMatch(%q,%q) error: %v", tc.pattern, tc.name, err)
		}
		if got != tc.want {
			t.Fatalf("globMatch(%q,%q) = %v, want %v", tc.pattern, tc.name, got, tc.want)
		}
	}
}

func TestScanInlineSuppressions(t *testing.T) {
	dir := t.TempDir()
	tf := filepath.Join(dir, "main.tf")
	body := `
# architex:ignore=s3_bucket_public_exposure reason="public docs site"
resource "aws_s3_bucket" "docs" {
  bucket = "x"
}

// architex:ignore=iam_admin_policy_attached reason="bootstrap role"
resource "aws_iam_role_policy_attachment" "admin" {
  role = "x"
}

# architex:ignore=foo
# architex:ignore=bar reason="stacked"
resource "aws_dynamodb_table" "t" {
  name = "x"
}

# architex:ignore=stale
output "ignored" {
  value = "x"
}
resource "aws_kinesis_stream" "k" {
  name = "x"
}
`
	if err := os.WriteFile(tf, []byte(body), 0o600); err != nil {
		t.Fatalf("write tf: %v", err)
	}
	got, err := ScanInlineSuppressions(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("expected 4 suppressions, got %d: %#v", len(got), got)
	}
	want := []struct {
		rule, res string
	}{
		{"s3_bucket_public_exposure", "aws_s3_bucket.docs"},
		{"iam_admin_policy_attached", "aws_iam_role_policy_attachment.admin"},
		{"foo", "aws_dynamodb_table.t"},
		{"bar", "aws_dynamodb_table.t"},
	}
	for i, w := range want {
		if got[i].Rule != w.rule || got[i].Resource != w.res {
			t.Fatalf("suppression %d = %s/%s, want %s/%s",
				i, got[i].Rule, got[i].Resource, w.rule, w.res)
		}
		if got[i].Source == "" {
			t.Fatalf("suppression %d missing source", i)
		}
		if got[i].Reason == "" {
			t.Fatalf("suppression %d missing default reason", i)
		}
	}
	for _, s := range got {
		if s.Rule == "stale" {
			t.Fatal("non-resource line must clear pending directives")
		}
	}
}
