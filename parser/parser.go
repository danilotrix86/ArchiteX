// Package parser reads .tf files and extracts resources and cross-resource
// references from HCL syntax. It handles a curated subset of Terraform
// constructs and logs warnings for anything unsupported.
package parser

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/hashicorp/hcl/v2/hclsyntax"

	"architex/models"
)

// ParseDir reads all .tf files in dir and returns extracted resources + warnings.
func ParseDir(dir string) ([]models.RawResource, []models.Warning, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("reading directory %s: %w", dir, err)
	}

	var allResources []models.RawResource
	var allWarnings []models.Warning

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".tf") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		resources, warnings, err := parseFile(path)
		if err != nil {
			allWarnings = append(allWarnings, models.Warning{
				Category: models.WarnParseError,
				Message:  fmt.Sprintf("failed to parse %s: %v", path, err),
			})
			continue
		}
		allResources = append(allResources, resources...)
		allWarnings = append(allWarnings, warnings...)
	}

	if len(allResources) == 0 {
		allWarnings = append(allWarnings, models.Warning{
			Category: models.WarnInfo,
			Message:  "no supported resources found in directory",
		})
	}

	return allResources, allWarnings, nil
}

func parseFile(path string) ([]models.RawResource, []models.Warning, error) {
	parser := hclparse.NewParser()
	file, diags := parser.ParseHCLFile(path)
	if diags.HasErrors() {
		return nil, nil, fmt.Errorf("HCL parse error: %s", diags.Error())
	}

	body, ok := file.Body.(*hclsyntax.Body)
	if !ok {
		return nil, nil, fmt.Errorf("unexpected body type in %s", path)
	}

	var resources []models.RawResource
	var warnings []models.Warning

	for _, block := range body.Blocks {
		switch block.Type {
		case "resource":
			res, warns := extractResource(block)
			if res != nil {
				resources = append(resources, *res)
			}
			warnings = append(warnings, warns...)

		case "module":
			warnings = append(warnings, models.Warning{
				Category: models.WarnUnsupportedConstruct,
				Message:  fmt.Sprintf("module block %q skipped (unsupported)", labelString(block.Labels)),
			})

		case "data", "variable", "output", "terraform", "provider", "locals":
			// Normal TF constructs that don't produce resources -- skip silently.

		default:
			warnings = append(warnings, models.Warning{
				Category: models.WarnUnsupportedConstruct,
				Message:  fmt.Sprintf("unknown block type %q skipped", block.Type),
			})
		}
	}

	return resources, warnings, nil
}

func extractResource(block *hclsyntax.Block) (*models.RawResource, []models.Warning) {
	if len(block.Labels) != 2 {
		return nil, []models.Warning{{
			Category: models.WarnUnsupportedConstruct,
			Message:  fmt.Sprintf("resource block with unexpected label count: %v", block.Labels),
		}}
	}

	resType := block.Labels[0]
	resName := block.Labels[1]
	resID := resType + "." + resName
	var warnings []models.Warning

	if !models.SupportedResources[resType] {
		warnings = append(warnings, models.Warning{
			Category: models.WarnUnsupportedResource,
			Message:  fmt.Sprintf("unsupported resource type %q (%s)", resType, resID),
		})
		return nil, warnings
	}

	for _, attr := range block.Body.Attributes {
		switch attr.Name {
		case "for_each", "count":
			warnings = append(warnings, models.Warning{
				Category: models.WarnUnsupportedConstruct,
				Message:  fmt.Sprintf("%s uses %q (unsupported, skipping resource)", resID, attr.Name),
			})
			return nil, warnings
		}
	}
	for _, nested := range block.Body.Blocks {
		if nested.Type == "dynamic" {
			warnings = append(warnings, models.Warning{
				Category: models.WarnUnsupportedConstruct,
				Message:  fmt.Sprintf("%s uses dynamic block (unsupported, skipping resource)", resID),
			})
			return nil, warnings
		}
	}

	attrs := extractAttributes(block.Body)
	refs := extractReferences(block.Body)

	return &models.RawResource{
		Type:       resType,
		Name:       resName,
		ID:         resID,
		Attributes: attrs,
		References: refs,
	}, warnings
}

func labelString(labels []string) string {
	return strings.Join(labels, ".")
}
