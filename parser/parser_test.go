package parser

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"architex/models"
)

const basicTF = `
resource "aws_vpc" "main" {
  cidr_block = "10.0.0.0/16"
}

resource "aws_subnet" "web" {
  vpc_id     = aws_vpc.main.id
  cidr_block = "10.0.1.0/24"
}

resource "aws_security_group" "web" {
  name   = "web-sg"
  vpc_id = aws_vpc.main.id

  ingress {
    from_port   = 80
    to_port     = 80
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

resource "aws_instance" "web" {
  ami                    = "ami-abc123"
  instance_type          = "t3.micro"
  subnet_id              = aws_subnet.web.id
  vpc_security_group_ids = [aws_security_group.web.id]
}
`

func writeTempTF(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte(content), 0644)
	if err != nil {
		t.Fatalf("writing temp tf: %v", err)
	}
	return dir
}

func findResource(resources []models.RawResource, id string) *models.RawResource {
	for i := range resources {
		if resources[i].ID == id {
			return &resources[i]
		}
	}
	return nil
}

func refTargets(r *models.RawResource) []string {
	targets := make([]string, 0, len(r.References))
	for _, ref := range r.References {
		targets = append(targets, ref.TargetID)
	}
	return targets
}

func warningsContain(warnings []models.Warning, substr string) bool {
	for _, w := range warnings {
		if strings.Contains(w.Message, substr) {
			return true
		}
	}
	return false
}

func warningsHaveCategory(warnings []models.Warning, category string) bool {
	for _, w := range warnings {
		if w.Category == category {
			return true
		}
	}
	return false
}

func TestParseDir_BasicResources(t *testing.T) {
	dir := writeTempTF(t, basicTF)
	resources, warnings, err := ParseDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("unexpected warnings: %v", warnings)
	}
	if len(resources) != 4 {
		t.Fatalf("expected 4 resources, got %d", len(resources))
	}

	ids := make(map[string]bool)
	for _, r := range resources {
		ids[r.ID] = true
	}

	expected := []string{
		"aws_vpc.main",
		"aws_subnet.web",
		"aws_security_group.web",
		"aws_instance.web",
	}
	for _, e := range expected {
		if !ids[e] {
			t.Errorf("missing resource %s", e)
		}
	}
}

func TestParseDir_References(t *testing.T) {
	dir := writeTempTF(t, basicTF)
	resources, _, err := ParseDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	instance := findResource(resources, "aws_instance.web")
	if instance == nil {
		t.Fatal("aws_instance.web not found")
	}
	instanceRefs := refTargets(instance)
	if !slices.Contains(instanceRefs, "aws_security_group.web") {
		t.Error("instance should reference aws_security_group.web")
	}
	if !slices.Contains(instanceRefs, "aws_subnet.web") {
		t.Error("instance should reference aws_subnet.web")
	}

	subnet := findResource(resources, "aws_subnet.web")
	if subnet == nil {
		t.Fatal("aws_subnet.web not found")
	}
	subnetRefs := refTargets(subnet)
	if !slices.Contains(subnetRefs, "aws_vpc.main") {
		t.Error("subnet should reference aws_vpc.main")
	}
}

func TestParseDir_CIDRBlocks(t *testing.T) {
	dir := writeTempTF(t, basicTF)
	resources, _, err := ParseDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sg := findResource(resources, "aws_security_group.web")
	if sg == nil {
		t.Fatal("aws_security_group.web not found")
	}
	cidrs, ok := sg.Attributes["cidr_blocks"]
	if !ok {
		t.Fatal("cidr_blocks not found on security group")
	}
	list, ok := cidrs.([]any)
	if !ok {
		t.Fatalf("cidr_blocks is %T, expected []any", cidrs)
	}
	if len(list) != 1 || list[0] != "0.0.0.0/0" {
		t.Errorf("unexpected cidr_blocks: %v", list)
	}
}

func TestParseDir_UnsupportedResource(t *testing.T) {
	// Note: aws_s3_bucket became supported in Phase 6 (v1.1). Use a type we
	// genuinely do not handle yet to exercise the unsupported-resource path.
	// aws_dynamodb_table is a deliberate next-Top-10 candidate that is NOT
	// yet in models.SupportedResources, so it is a stable choice here.
	tf := `
resource "aws_dynamodb_table" "sessions" {
  name         = "sessions"
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "id"
}

resource "aws_vpc" "main" {
  cidr_block = "10.0.0.0/16"
}
`
	dir := writeTempTF(t, tf)
	resources, warnings, err := ParseDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resources) != 1 {
		t.Errorf("expected 1 supported resource, got %d", len(resources))
	}
	if !warningsContain(warnings, "unsupported resource type") {
		t.Error("expected 'unsupported resource type' warning")
	}
	if !warningsHaveCategory(warnings, models.WarnUnsupportedResource) {
		t.Errorf("expected category %q, got categories %v",
			models.WarnUnsupportedResource, warningCategories(warnings))
	}
}

func warningCategories(warnings []models.Warning) []string {
	cats := make([]string, 0, len(warnings))
	for _, w := range warnings {
		cats = append(cats, w.Category)
	}
	return cats
}

// TestParseDir_ForEachUnresolvableSkipped verifies that for_each over an
// unresolvable expression (a variable, in this case) still produces no
// resources and an unsupported_construct warning -- the v1.0/v1.1 contract
// is preserved for the case where Phase 7 cannot safely expand.
func TestParseDir_ForEachUnresolvableSkipped(t *testing.T) {
	tf := `
resource "aws_instance" "multi" {
  for_each      = var.targets
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
		t.Errorf("expected 0 resources (var.targets is unresolvable), got %d", len(resources))
	}
	if !warningsHaveCategory(warnings, models.WarnUnsupportedConstruct) {
		t.Error("expected unsupported_construct warning for non-literal for_each")
	}
}

// TestParseDir_RemoteModuleWarning verifies that remote module sources are
// NOT fetched (we never introduce a new outbound network surface) and emit
// an unsupported_construct warning instead.
func TestParseDir_RemoteModuleWarning(t *testing.T) {
	tf := `
module "vpc" {
  source = "terraform-aws-modules/vpc/aws"
}
`
	dir := writeTempTF(t, tf)
	resources, warnings, err := ParseDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resources) != 0 {
		t.Errorf("expected 0 resources for remote module, got %d", len(resources))
	}
	if !warningsContain(warnings, "remote source") {
		t.Errorf("expected warning about remote source; got %v", warnings)
	}
}

// TestParseDir_Phase6Resources_AllRecognized verifies that every Phase 6
// (v1.1 "AWS Top 10") resource type is recognized by the parser without
// emitting `unsupported_resource` warnings. The companion check on the
// graph layer (TestBuild_Phase6_DerivedAttributesAndEdges) confirms that
// each abstract type and edge type is wired through correctly.
func TestParseDir_Phase6Resources_AllRecognized(t *testing.T) {
	resources, warnings, err := ParseDir("../testdata/top10_resources")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, w := range warnings {
		if w.Category == models.WarnUnsupportedResource {
			t.Errorf("unexpected unsupported_resource warning: %s", w.Message)
		}
	}

	expected := []string{
		"aws_s3_bucket.logs",
		"aws_s3_bucket_public_access_block.logs",
		"aws_s3_bucket_policy.logs",
		"aws_iam_role.lambda_exec",
		"aws_iam_policy.read_only",
		"aws_iam_role_policy_attachment.lambda_read_only",
		"aws_lambda_function.worker",
		"aws_lambda_function_url.worker",
		"aws_apigatewayv2_api.http",
		"aws_internet_gateway.main",
	}
	for _, id := range expected {
		if findResource(resources, id) == nil {
			t.Errorf("Phase 6 resource %s was not parsed", id)
		}
	}
}

// TestParseDir_Tranche2Resources_AllRecognized verifies that every Phase 7
// PR4 (v1.2 "Coverage tranche 2") resource type is recognized by the parser
// without emitting `unsupported_resource` warnings. Mirrors the equivalent
// Phase 6 test above.
func TestParseDir_Tranche2Resources_AllRecognized(t *testing.T) {
	resources, warnings, err := ParseDir("../testdata/tranche2_resources")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, w := range warnings {
		if w.Category == models.WarnUnsupportedResource {
			t.Errorf("unexpected unsupported_resource warning: %s", w.Message)
		}
	}

	expected := []string{
		"aws_cloudfront_distribution.web",
		"aws_route53_zone.main",
		"aws_route53_record.www",
		"aws_kms_key.main",
		"aws_kms_alias.main",
		"aws_sns_topic.alerts",
		"aws_sns_topic_policy.alerts",
		"aws_sqs_queue.jobs",
		"aws_sqs_queue_policy.jobs",
		"aws_nat_gateway.main",
		"aws_network_acl.main",
		"aws_network_acl_rule.open_inbound",
		"aws_secretsmanager_secret.db",
		"aws_ebs_volume.data",
		"aws_ecs_cluster.main",
		"aws_ecs_task_definition.app",
		"aws_ecs_service.app",
	}
	for _, id := range expected {
		if findResource(resources, id) == nil {
			t.Errorf("tranche-2 resource %s was not parsed", id)
		}
	}
}

// TestParseDir_Tranche2_LiteralAttributesPromoted spot-checks that the four
// new rules' input attributes survive the parser round-trip:
//   - aws_ebs_volume.encrypted (bool literal)
//   - aws_network_acl_rule.{cidr_block, egress, rule_action} (mixed literals)
//   - aws_sns_topic_policy.policy (jsonencode -> JSON string)
//
// The aws_cloudfront_distribution.web_acl_id literal is exercised end-to-end
// by the cloudfront_no_waf rule tests in risk/rules_v12_test.go.
func TestParseDir_Tranche2_LiteralAttributesPromoted(t *testing.T) {
	resources, _, err := ParseDir("../testdata/tranche2_resources")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ebs := findResource(resources, "aws_ebs_volume.data")
	if ebs == nil {
		t.Fatal("aws_ebs_volume.data not parsed")
	}
	if got, ok := ebs.Attributes["encrypted"].(bool); !ok || got {
		t.Errorf("expected encrypted=false (bool), got %v (%T)", ebs.Attributes["encrypted"], ebs.Attributes["encrypted"])
	}

	nacl := findResource(resources, "aws_network_acl_rule.open_inbound")
	if nacl == nil {
		t.Fatal("aws_network_acl_rule.open_inbound not parsed")
	}
	if got, _ := nacl.Attributes["cidr_block"].(string); got != "0.0.0.0/0" {
		t.Errorf("expected cidr_block=0.0.0.0/0, got %v", nacl.Attributes["cidr_block"])
	}
	if got, _ := nacl.Attributes["rule_action"].(string); got != "allow" {
		t.Errorf("expected rule_action=allow, got %v", nacl.Attributes["rule_action"])
	}
	if got, ok := nacl.Attributes["egress"].(bool); !ok || got {
		t.Errorf("expected egress=false (bool), got %v (%T)", nacl.Attributes["egress"], nacl.Attributes["egress"])
	}

	pol := findResource(resources, "aws_sns_topic_policy.alerts")
	if pol == nil {
		t.Fatal("aws_sns_topic_policy.alerts not parsed")
	}
	raw, ok := pol.Attributes["policy"].(string)
	if !ok || raw == "" {
		t.Fatalf("expected jsonencode-resolved policy string, got %v", pol.Attributes["policy"])
	}
	if !strings.Contains(raw, `"Effect":"Allow"`) || !strings.Contains(raw, `"Principal":"*"`) {
		t.Errorf("policy JSON missing expected fragments, got: %s", raw)
	}
}

// TestParseDir_Phase6_PolicyArnLiteralCaptured ensures the parser preserves
// the literal `policy_arn` string on aws_iam_role_policy_attachment, which
// the Phase 6 iam_admin_policy_attached rule will key off.
func TestParseDir_Phase6_PolicyArnLiteralCaptured(t *testing.T) {
	tf := `
resource "aws_iam_role" "app" {
  name               = "x"
  assume_role_policy = "{}"
}

resource "aws_iam_role_policy_attachment" "admin" {
  role       = aws_iam_role.app.name
  policy_arn = "arn:aws:iam::aws:policy/AdministratorAccess"
}
`
	dir := writeTempTF(t, tf)
	resources, _, err := ParseDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	att := findResource(resources, "aws_iam_role_policy_attachment.admin")
	if att == nil {
		t.Fatal("aws_iam_role_policy_attachment.admin not found")
	}
	got, ok := att.Attributes["policy_arn"].(string)
	if !ok {
		t.Fatalf("policy_arn was %T (%v), expected string literal", att.Attributes["policy_arn"], att.Attributes["policy_arn"])
	}
	if !strings.HasSuffix(got, "AdministratorAccess") {
		t.Errorf("expected policy_arn to retain AdministratorAccess suffix, got %q", got)
	}
}

func TestParseDir_FilterVarLocalReferences(t *testing.T) {
	tf := `
resource "aws_instance" "web" {
  ami           = var.ami_id
  instance_type = local.instance_type
  subnet_id     = aws_subnet.web.id
}

resource "aws_subnet" "web" {
  vpc_id     = aws_vpc.main.id
  cidr_block = "10.0.1.0/24"
}
`
	dir := writeTempTF(t, tf)
	resources, _, err := ParseDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	instance := findResource(resources, "aws_instance.web")
	if instance == nil {
		t.Fatal("aws_instance.web not found")
	}
	for _, ref := range instance.References {
		if ref.TargetID == "var.ami_id" || ref.TargetID == "local.instance_type" {
			t.Errorf("should not include reference to %s", ref.TargetID)
		}
	}
	if len(instance.References) != 1 {
		t.Errorf("expected 1 reference, got %d", len(instance.References))
	}
	if instance.References[0].TargetID != "aws_subnet.web" {
		t.Errorf("expected reference to aws_subnet.web, got %s", instance.References[0].TargetID)
	}
}
