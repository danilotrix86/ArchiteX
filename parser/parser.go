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

	"architex/config"
	"architex/models"
)

// maxModuleDepth bounds recursive local-module expansion so a malicious or
// accidental module cycle cannot wedge the parser. The limit is generous
// (real-world module trees are typically <= 3 deep) and the warning is
// auditable.
const maxModuleDepth = 8

// ParseDir reads all .tf files in dir and returns extracted resources + warnings.
//
// Phase 7 (v1.2 PR3): use ParseDirWith to pass an optional `*config.Config`
// whose ignore.paths patterns will skip matching .tf files. ParseDir keeps
// the v1.0/v1.1 zero-config signature unchanged.
func ParseDir(dir string) ([]models.RawResource, []models.Warning, error) {
	return ParseDirWith(dir, nil)
}

// ParseDirWith is the configurable variant of ParseDir. A nil cfg reproduces
// v1.1 behavior exactly. When cfg is set, files relative to `dir` whose
// path matches any ignore.paths pattern are skipped (and the absent file
// emits no warning at all -- the user explicitly opted out).
func ParseDirWith(dir string, cfg *config.Config) ([]models.RawResource, []models.Warning, error) {
	return parseDirAt(dir, dir, "", 0, cfg)
}

// parseDirAt is the recursive worker behind ParseDir. The `idPrefix` is
// prepended to every produced resource ID so resources inside local modules
// are namespaced (`module.<name>.aws_x.y`) and never collide with resources
// in the calling directory. depth tracks recursion to enforce
// maxModuleDepth. `rootDir` is the original top-level directory and is the
// anchor for relative-path matching against cfg.Ignore.Paths.
func parseDirAt(dir, rootDir, idPrefix string, depth int, cfg *config.Config) ([]models.RawResource, []models.Warning, error) {
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
		if cfg != nil {
			rel, _ := filepath.Rel(rootDir, path)
			if cfg.IsPathIgnored(rel) {
				// User explicitly excluded this file -- do not even read it.
				continue
			}
		}
		resources, warnings, err := parseFile(path, dir, rootDir, idPrefix, depth, cfg)
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

	if len(allResources) == 0 && idPrefix == "" {
		// Only emit the "empty directory" info-warning at the top level.
		// A genuinely empty submodule is a normal degenerate case during
		// expansion (e.g. an outputs-only module) and doesn't merit a
		// warning at every depth.
		allWarnings = append(allWarnings, models.Warning{
			Category: models.WarnInfo,
			Message:  "no supported resources found in directory",
		})
	}

	return allResources, allWarnings, nil
}

func parseFile(path, dir, rootDir, idPrefix string, depth int, cfg *config.Config) ([]models.RawResource, []models.Warning, error) {
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

	// Phase 7: pre-scan the file for data blocks we know how to resolve.
	// This must happen before resource expansion so attribute extraction
	// can substitute `data.aws_iam_policy.<name>.arn` references with the
	// captured literal regardless of declaration order.
	ctx := newParseContext()
	scanDataBlocks(body, ctx)

	for _, block := range body.Blocks {
		switch block.Type {
		case "resource":
			expanded, warns := expandResource(block, ctx)
			for i := range expanded {
				if idPrefix != "" {
					expanded[i].ID = idPrefix + expanded[i].ID
				}
			}
			resources = append(resources, expanded...)
			warnings = append(warnings, warns...)

		case "module":
			modResources, modWarnings := expandModule(block, dir, rootDir, idPrefix, depth, cfg)
			resources = append(resources, modResources...)
			warnings = append(warnings, modWarnings...)

		case "data", "variable", "output", "terraform", "provider", "locals":
			// Normal TF constructs that don't produce resources -- skip silently.
			// `data` blocks may have already contributed to `ctx` above; the
			// silent-skip here is the v1.0/v1.1 contract for everything else.

		default:
			warnings = append(warnings, models.Warning{
				Category: models.WarnUnsupportedConstruct,
				Message:  fmt.Sprintf("unknown block type %q skipped", block.Type),
			})
		}
	}

	return resources, warnings, nil
}

// expandModule handles a `module "name" { source = "..." }` block.
//
// Local sources (`./...` or `../...`) are recursively parsed; their resources
// are returned with IDs namespaced as `module.<name>.<original_id>` so they
// participate in the graph as first-class nodes alongside top-level resources.
//
// Remote sources (registry, git::, http://, etc.) keep emitting an
// unsupported_construct warning -- fetching them would introduce a network
// surface and Phase 7 is intentionally local-only.
func expandModule(block *hclsyntax.Block, dir, rootDir, idPrefix string, depth int, cfg *config.Config) ([]models.RawResource, []models.Warning) {
	if len(block.Labels) != 1 {
		return nil, []models.Warning{{
			Category: models.WarnUnsupportedConstruct,
			Message:  fmt.Sprintf("module block with unexpected label count: %v", block.Labels),
		}}
	}
	modName := block.Labels[0]

	if depth >= maxModuleDepth {
		return nil, []models.Warning{{
			Category: models.WarnUnsupportedConstruct,
			Message: fmt.Sprintf("module %q exceeds max recursion depth %d (cycle?); skipping",
				modName, maxModuleDepth),
		}}
	}

	srcAttr, ok := block.Body.Attributes["source"]
	if !ok {
		return nil, []models.Warning{{
			Category: models.WarnUnsupportedConstruct,
			Message:  fmt.Sprintf("module %q missing source attribute; skipping", modName),
		}}
	}

	srcVal, srcDiags := srcAttr.Expr.Value(nil)
	if srcDiags.HasErrors() || !srcVal.IsKnown() || srcVal.IsNull() || srcVal.Type().FriendlyName() != "string" {
		return nil, []models.Warning{{
			Category: models.WarnUnsupportedConstruct,
			Message:  fmt.Sprintf("module %q has non-literal source; skipping", modName),
		}}
	}
	source := srcVal.AsString()

	if !isLocalModuleSource(source) {
		// Remote sources are intentionally not fetched -- that would be a
		// new outbound network surface (master.md design decision 27).
		return nil, []models.Warning{{
			Category: models.WarnUnsupportedConstruct,
			Message: fmt.Sprintf("module %q uses remote source %q (not fetched; runner-local trust model)",
				modName, source),
		}}
	}

	// Resolve the local source relative to the calling directory.
	modDir := source
	if !filepath.IsAbs(modDir) {
		modDir = filepath.Join(dir, source)
	}

	// Recurse with the module name appended to the ID prefix.
	childPrefix := idPrefix + "module." + modName + "."
	resources, warnings, err := parseDirAt(modDir, rootDir, childPrefix, depth+1, cfg)
	if err != nil {
		return nil, []models.Warning{{
			Category: models.WarnUnsupportedConstruct,
			Message:  fmt.Sprintf("module %q (source %q) failed to parse: %v", modName, source, err),
		}}
	}

	return resources, warnings
}

// isLocalModuleSource returns true for module sources that point at a path
// on the local filesystem (`./...`, `../...`, or an absolute path).
// Everything else (registry, git::, https://, s3::, hg::, etc.) is treated
// as remote and skipped.
func isLocalModuleSource(source string) bool {
	if source == "" {
		return false
	}
	if strings.HasPrefix(source, "./") || strings.HasPrefix(source, "../") {
		return true
	}
	if strings.HasPrefix(source, ".\\") || strings.HasPrefix(source, "..\\") {
		return true
	}
	if filepath.IsAbs(source) {
		return true
	}
	return false
}
