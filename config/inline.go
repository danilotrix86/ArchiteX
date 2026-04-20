package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ---------------------------------------------------------------------------
// Inline suppressions: `# architex:ignore=<rule_id> reason="<text>"` directives
// placed on the line(s) immediately preceding a `resource "type" "name" {`
// block. Each directive associates with the very next resource block in the
// same file. Multiple directives on consecutive lines stack.
//
// We deliberately do NOT use the HCL AST for this -- comments are not part of
// the parsed AST, and reading the .tf as text is both simpler and faster.
// The format is line-oriented so that authors can spot the directive at the
// site of the resource without diving into config files.
// ---------------------------------------------------------------------------

// inlineDirective matches:
//
//	# architex:ignore=rule_id
//	# architex:ignore=rule_id reason="text"
//	// architex:ignore=rule_id reason="text"
//
// `rule_id` is matched greedily up to the first whitespace.
var inlineDirective = regexp.MustCompile(
	`(?:#|//)\s*architex:ignore\s*=\s*([A-Za-z0-9_]+)\s*(?:reason\s*=\s*"([^"]*)")?`,
)

// resourceLine matches `resource "type" "name" {` (capturing groups 1 and 2).
var resourceLine = regexp.MustCompile(
	`^\s*resource\s+"([^"]+)"\s+"([^"]+)"`,
)

// ScanInlineSuppressions walks one or more directories for .tf files and
// extracts inline suppressions. Directories that don't exist are skipped
// silently (the caller may pass speculative paths). Returns a flat slice
// suitable for Config.Add.
//
// Inline suppressions get a synthetic Source label like
//
//	"inline:infra/main.tf:42"
//
// so the audit-bundle suppressions footer can point a reviewer at the
// originating line.
func ScanInlineSuppressions(dirs ...string) ([]Suppression, error) {
	var out []Suppression
	for _, dir := range dirs {
		ss, err := scanDir(dir)
		if err != nil {
			return nil, err
		}
		out = append(out, ss...)
	}
	return out, nil
}

func scanDir(dir string) ([]Suppression, error) {
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		// Allow callers to pass a single .tf file too.
		if strings.HasSuffix(dir, ".tf") {
			return scanFile(dir)
		}
		return nil, nil
	}

	var out []Suppression
	walkErr := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			// Skip hidden dirs (.git, .terraform) for performance and
			// because they never contain user-authored .tf files.
			base := filepath.Base(path)
			if base != "." && strings.HasPrefix(base, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(info.Name(), ".tf") {
			return nil
		}
		ss, err := scanFile(path)
		if err != nil {
			return err
		}
		out = append(out, ss...)
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	return out, nil
}

func scanFile(path string) ([]Suppression, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	type pending struct {
		rule, reason string
		line         int
	}

	var (
		out      []Suppression
		pendings []pending
		lineNum  int
	)

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if m := inlineDirective.FindStringSubmatch(trimmed); m != nil {
			pendings = append(pendings, pending{
				rule:   m[1],
				reason: m[2],
				line:   lineNum,
			})
			continue
		}

		if rm := resourceLine.FindStringSubmatch(trimmed); rm != nil {
			resID := rm[1] + "." + rm[2]
			for _, p := range pendings {
				reason := p.reason
				if reason == "" {
					reason = "inline ignore (no reason supplied)"
				}
				out = append(out, Suppression{
					Rule:     p.rule,
					Resource: resID,
					Reason:   reason,
					Source:   fmt.Sprintf("inline:%s:%d", path, p.line),
				})
			}
			pendings = nil
			continue
		}

		// Non-empty, non-directive, non-resource line clears any pending
		// directives -- they only attach to the immediately-following
		// resource block.
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") && !strings.HasPrefix(trimmed, "//") {
			pendings = nil
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan %s: %w", path, err)
	}
	return out, nil
}
