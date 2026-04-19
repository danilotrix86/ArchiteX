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
	tf := `
resource "aws_s3_bucket" "logs" {
  bucket = "my-logs"
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

func TestParseDir_ForEachSkipped(t *testing.T) {
	tf := `
resource "aws_instance" "multi" {
  for_each      = toset(["a", "b"])
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
		t.Errorf("expected 0 resources (for_each should be skipped), got %d", len(resources))
	}
	if len(warnings) == 0 {
		t.Error("expected warning for for_each usage")
	}
}

func TestParseDir_ModuleWarning(t *testing.T) {
	tf := `
module "vpc" {
  source = "./modules/vpc"
}
`
	dir := writeTempTF(t, tf)
	resources, warnings, err := ParseDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resources) != 0 {
		t.Errorf("expected 0 resources, got %d", len(resources))
	}
	if !warningsContain(warnings, "module") {
		t.Error("expected warning about module block")
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
