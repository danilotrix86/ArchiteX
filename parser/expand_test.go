package parser

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"architex/models"
)

// ---------------------------------------------------------------------------
// Phase 7 (v1.2) -- parser-depth expansion tests.
//
// These tests pin the contract that v1.0/v1.1 fixtures keep producing the
// same resources, AND that count / for_each / dynamic / local-module inputs
// expand into the deterministic per-instance shapes the rest of the engine
// expects.
// ---------------------------------------------------------------------------

// TestParseDir_CountLiteral verifies that `count = <int literal>` expands
// into N RawResources with deterministic instance suffixes.
func TestParseDir_CountLiteral(t *testing.T) {
	tf := `
resource "aws_instance" "worker" {
  count         = 3
  ami           = "ami-abc123"
  instance_type = "t3.micro"
}
`
	dir := writeTempTF(t, tf)
	resources, warnings, err := ParseDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("unexpected warnings: %v", warnings)
	}
	if len(resources) != 3 {
		t.Fatalf("expected 3 expanded instances, got %d", len(resources))
	}
	expectedIDs := []string{
		"aws_instance.worker[0]",
		"aws_instance.worker[1]",
		"aws_instance.worker[2]",
	}
	gotIDs := collectResourceIDs(resources)
	for _, want := range expectedIDs {
		if !contains(gotIDs, want) {
			t.Errorf("missing expanded resource %q (got %v)", want, gotIDs)
		}
	}
}

// TestParseDir_CountZero verifies the Terraform-conformant behavior that
// `count = 0` produces no instances and no warnings.
func TestParseDir_CountZero(t *testing.T) {
	tf := `
resource "aws_instance" "worker" {
  count         = 0
  ami           = "ami-abc123"
  instance_type = "t3.micro"
}
`
	dir := writeTempTF(t, tf)
	resources, warnings, err := ParseDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resources) != 0 {
		t.Errorf("count=0 should produce 0 resources, got %d", len(resources))
	}
	for _, w := range warnings {
		if w.Category == models.WarnUnsupportedConstruct {
			t.Errorf("count=0 must not produce unsupported_construct warnings, got %q", w.Message)
		}
	}
}

// TestParseDir_CountLengthLiteral verifies the canonical
// `count = length([literal_list])` idiom expands by the list's length.
func TestParseDir_CountLengthLiteral(t *testing.T) {
	tf := `
resource "aws_instance" "worker" {
  count         = length(["a", "b", "c", "d"])
  ami           = "ami-abc123"
  instance_type = "t3.micro"
}
`
	dir := writeTempTF(t, tf)
	resources, warnings, err := ParseDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("unexpected warnings: %v", warnings)
	}
	if len(resources) != 4 {
		t.Errorf("expected 4 expanded instances from length(literal), got %d", len(resources))
	}
}

// TestParseDir_CountVariableSkipped verifies that `count = var.x` continues
// to warn-and-skip, mirroring v1.0/v1.1 behavior for unresolvable inputs.
func TestParseDir_CountVariableSkipped(t *testing.T) {
	tf := `
resource "aws_instance" "worker" {
  count         = var.replica_count
  ami           = "ami-abc123"
  instance_type = "t3.micro"
}
`
	dir := writeTempTF(t, tf)
	resources, warnings, err := ParseDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resources) != 0 {
		t.Errorf("expected 0 resources for var-driven count, got %d", len(resources))
	}
	if !warningsHaveCategory(warnings, models.WarnUnsupportedConstruct) {
		t.Error("expected unsupported_construct warning for var-driven count")
	}
}

// TestParseDir_ForEachLiteralSet verifies that `for_each = toset(["a", "b"])`
// expands into per-key instances in deterministic sorted order.
func TestParseDir_ForEachLiteralSet(t *testing.T) {
	tf := `
resource "aws_instance" "worker" {
  for_each      = toset(["staging", "prod", "dev"])
  ami           = "ami-abc123"
  instance_type = "t3.micro"
}
`
	dir := writeTempTF(t, tf)
	resources, warnings, err := ParseDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("unexpected warnings: %v", warnings)
	}
	if len(resources) != 3 {
		t.Fatalf("expected 3 expanded instances, got %d", len(resources))
	}
	want := []string{
		`aws_instance.worker["dev"]`,
		`aws_instance.worker["prod"]`,
		`aws_instance.worker["staging"]`,
	}
	gotIDs := collectResourceIDs(resources)
	sort.Strings(gotIDs)
	for i, w := range want {
		if gotIDs[i] != w {
			t.Errorf("for_each instance %d: expected %q, got %q", i, w, gotIDs[i])
		}
	}
}

// TestParseDir_ForEachLiteralMap verifies that `for_each = { ... literal map }`
// uses the map keys as instance keys.
func TestParseDir_ForEachLiteralMap(t *testing.T) {
	tf := `
resource "aws_instance" "worker" {
  for_each = {
    "alpha" = "ami-1"
    "beta"  = "ami-2"
  }
  ami           = each.value
  instance_type = "t3.micro"
}
`
	dir := writeTempTF(t, tf)
	resources, _, err := ParseDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resources) != 2 {
		t.Fatalf("expected 2 instances from literal map, got %d", len(resources))
	}
	gotIDs := collectResourceIDs(resources)
	sort.Strings(gotIDs)
	want := []string{`aws_instance.worker["alpha"]`, `aws_instance.worker["beta"]`}
	for i, w := range want {
		if gotIDs[i] != w {
			t.Errorf("for_each map instance %d: expected %q, got %q", i, w, gotIDs[i])
		}
	}
}

// TestParseDir_DynamicBlockLiteral verifies that a dynamic block with a
// literal for_each expands into N concrete nested blocks. This is the
// real-world idiom for security_group dynamic ingress rules.
func TestParseDir_DynamicBlockLiteral(t *testing.T) {
	tf := `
resource "aws_security_group" "web" {
  name = "web"

  dynamic "ingress" {
    for_each = toset(["80", "443"])
    content {
      from_port   = 0
      to_port     = 0
      protocol    = "tcp"
      cidr_blocks = ["0.0.0.0/0"]
    }
  }
}
`
	dir := writeTempTF(t, tf)
	resources, warnings, err := ParseDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("unexpected warnings: %v", warnings)
	}
	if len(resources) != 1 {
		t.Fatalf("expected 1 SG, got %d", len(resources))
	}
	sg := resources[0]
	cidrs, ok := sg.Attributes["cidr_blocks"]
	if !ok {
		t.Fatal("dynamic ingress should have promoted cidr_blocks to top level")
	}
	list, ok := cidrs.([]any)
	if !ok || len(list) == 0 {
		t.Fatalf("expected non-empty cidr_blocks slice, got %T %v", cidrs, cidrs)
	}
	for _, item := range list {
		if s, ok := item.(string); !ok || s != "0.0.0.0/0" {
			t.Errorf("dynamic-expanded cidr should be 0.0.0.0/0, got %v", item)
		}
	}
}

// TestParseDir_DynamicBlockUnresolvable verifies that a dynamic block whose
// for_each is unresolvable warns + drops the block but PRESERVES the
// surrounding resource (a v1.0/v1.1 contract change: previously the entire
// resource was skipped).
func TestParseDir_DynamicBlockUnresolvable(t *testing.T) {
	tf := `
resource "aws_security_group" "web" {
  name = "web"

  dynamic "ingress" {
    for_each = var.ingress_rules
    content {
      from_port   = 0
      to_port     = 0
      cidr_blocks = ["0.0.0.0/0"]
    }
  }
}
`
	dir := writeTempTF(t, tf)
	resources, warnings, err := ParseDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resources) != 1 {
		t.Errorf("dynamic with unresolvable for_each should still produce the SG, got %d resources",
			len(resources))
	}
	if !warningsHaveCategory(warnings, models.WarnUnsupportedConstruct) {
		t.Error("expected unsupported_construct warning for unresolvable dynamic for_each")
	}
}

// TestParseDir_LocalModuleRecursion verifies that a `module` block with a
// local source ("./submodule") recursively parses the submodule and emits
// its resources with namespaced IDs.
func TestParseDir_LocalModuleRecursion(t *testing.T) {
	root := t.TempDir()
	subDir := filepath.Join(root, "vpc")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("mkdir submodule: %v", err)
	}

	rootTF := `
module "vpc" {
  source = "./vpc"
}
`
	subTF := `
resource "aws_vpc" "main" {
  cidr_block = "10.0.0.0/16"
}

resource "aws_subnet" "private" {
  vpc_id     = aws_vpc.main.id
  cidr_block = "10.0.1.0/24"
}
`
	if err := os.WriteFile(filepath.Join(root, "main.tf"), []byte(rootTF), 0o644); err != nil {
		t.Fatalf("write root tf: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "vpc.tf"), []byte(subTF), 0o644); err != nil {
		t.Fatalf("write sub tf: %v", err)
	}

	resources, warnings, err := ParseDir(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, w := range warnings {
		if w.Category == models.WarnUnsupportedConstruct {
			t.Errorf("local module recursion should not warn; got %q", w.Message)
		}
	}
	if len(resources) != 2 {
		t.Fatalf("expected 2 namespaced resources, got %d", len(resources))
	}

	gotIDs := collectResourceIDs(resources)
	want := []string{"module.vpc.aws_vpc.main", "module.vpc.aws_subnet.private"}
	for _, w := range want {
		if !contains(gotIDs, w) {
			t.Errorf("missing namespaced resource %q (got %v)", w, gotIDs)
		}
	}
}

// TestParseDir_NestedLocalModule_DepthChain verifies module-in-module
// recursion is supported up to maxModuleDepth and that IDs are namespaced
// at each level.
func TestParseDir_NestedLocalModule_DepthChain(t *testing.T) {
	root := t.TempDir()
	mid := filepath.Join(root, "mid")
	leaf := filepath.Join(mid, "leaf")
	if err := os.MkdirAll(leaf, 0o755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}

	mustWrite := func(p, body string) {
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", p, err)
		}
	}

	mustWrite(filepath.Join(root, "main.tf"), `
module "mid" { source = "./mid" }
`)
	mustWrite(filepath.Join(mid, "main.tf"), `
module "leaf" { source = "./leaf" }
`)
	mustWrite(filepath.Join(leaf, "main.tf"), `
resource "aws_vpc" "deep" { cidr_block = "10.0.0.0/16" }
`)

	resources, _, err := ParseDir(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resources) != 1 {
		t.Fatalf("expected 1 deeply-namespaced resource, got %d", len(resources))
	}
	want := "module.mid.module.leaf.aws_vpc.deep"
	if resources[0].ID != want {
		t.Errorf("expected ID %q, got %q", want, resources[0].ID)
	}
}

// TestParseDir_RegressionV1Fixtures locks in the contract that PR1 adds
// expansion WITHOUT changing v1.0/v1.1 behavior on existing fixtures. If
// any of these counts drift, the canonical 9.0/10 score regression test
// will follow -- catching this at the parser layer is faster.
func TestParseDir_RegressionV1Fixtures(t *testing.T) {
	cases := []struct {
		dir          string
		minResources int
	}{
		{"../testdata/base", 5},
		{"../testdata/head", 6},
		{"../testdata/db_added_base", 1},
		{"../testdata/db_added_head", 2},
		{"../testdata/removed_base", 2},
		{"../testdata/removed_head", 1},
		{"../testdata/top10_base", 5},
		{"../testdata/top10_head", 5},
	}
	for _, c := range cases {
		t.Run(c.dir, func(t *testing.T) {
			resources, _, err := ParseDir(c.dir)
			if err != nil {
				t.Fatalf("ParseDir(%s): %v", c.dir, err)
			}
			if len(resources) < c.minResources {
				t.Errorf("ParseDir(%s) = %d resources, want >= %d (v1 fixtures must not regress)",
					c.dir, len(resources), c.minResources)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Phase 7 (v1.2 PR2) -- caveat fixes: data-source resolution + jsonencode.
// ---------------------------------------------------------------------------

// TestParseDir_DataPolicyARN_ResolvedFromDataBlock verifies that
// `policy_arn = data.aws_iam_policy.<name>.arn` is substituted with the
// literal ARN captured from the matching data block, regardless of
// declaration order in the file.
func TestParseDir_DataPolicyARN_ResolvedFromDataBlock(t *testing.T) {
	tf := `
resource "aws_iam_role" "app" {
  name               = "app"
  assume_role_policy = "{}"
}

resource "aws_iam_role_policy_attachment" "admin" {
  role       = aws_iam_role.app.name
  policy_arn = data.aws_iam_policy.admin.arn
}

data "aws_iam_policy" "admin" {
  arn = "arn:aws:iam::aws:policy/AdministratorAccess"
}
`
	dir := writeTempTF(t, tf)
	resources, _, err := ParseDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	att := findResource(resources, "aws_iam_role_policy_attachment.admin")
	if att == nil {
		t.Fatal("attachment not found")
	}
	got, ok := att.Attributes["policy_arn"].(string)
	if !ok {
		t.Fatalf("PR2: expected resolved literal policy_arn, got %T %v",
			att.Attributes["policy_arn"], att.Attributes["policy_arn"])
	}
	want := "arn:aws:iam::aws:policy/AdministratorAccess"
	if got != want {
		t.Errorf("expected policy_arn %q, got %q", want, got)
	}
}

// TestParseDir_DataPolicyARN_UnknownDataSource_StaysNil verifies that we do
// not invent a resolution for an attachment whose data source isn't in the
// pre-scanned set. The attribute must remain unresolved (nil) and the rule
// must not fire downstream -- preserving design decision 14.
func TestParseDir_DataPolicyARN_UnknownDataSource_StaysNil(t *testing.T) {
	tf := `
resource "aws_iam_role" "app" {
  name               = "app"
  assume_role_policy = "{}"
}

resource "aws_iam_role_policy_attachment" "x" {
  role       = aws_iam_role.app.name
  policy_arn = data.aws_iam_policy.unknown.arn
}
`
	dir := writeTempTF(t, tf)
	resources, _, err := ParseDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	att := findResource(resources, "aws_iam_role_policy_attachment.x")
	if att == nil {
		t.Fatal("attachment not found")
	}
	if v := att.Attributes["policy_arn"]; v != nil {
		t.Errorf("PR2: unknown data source must NOT be guessed; got %v", v)
	}
}

// TestParseDir_S3Policy_JSONEncodeLiteral verifies that
// `policy = jsonencode({...literal...})` resolves to a JSON string the
// downstream rule can inspect for Effect="Deny".
func TestParseDir_S3Policy_JSONEncodeLiteral(t *testing.T) {
	tf := `
resource "aws_s3_bucket" "logs" {
  bucket = "logs"
}

resource "aws_s3_bucket_policy" "logs" {
  bucket = aws_s3_bucket.logs.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Deny"
      Principal = "*"
      Action    = "s3:*"
      Resource  = "*"
    }]
  })
}
`
	dir := writeTempTF(t, tf)
	resources, _, err := ParseDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	bp := findResource(resources, "aws_s3_bucket_policy.logs")
	if bp == nil {
		t.Fatal("aws_s3_bucket_policy.logs not found")
	}
	policy, ok := bp.Attributes["policy"].(string)
	if !ok || policy == "" {
		t.Fatalf("PR2: jsonencode(literal) should resolve to a JSON string; got %T %v",
			bp.Attributes["policy"], bp.Attributes["policy"])
	}
	if !strings.Contains(policy, `"Effect":"Deny"`) {
		t.Errorf("expected resolved policy to contain Deny effect; got %q", policy)
	}
}

func collectResourceIDs(resources []models.RawResource) []string {
	ids := make([]string, 0, len(resources))
	for _, r := range resources {
		ids = append(ids, r.ID)
	}
	return ids
}

func contains(slice []string, want string) bool {
	for _, s := range slice {
		if s == want {
			return true
		}
	}
	return false
}
