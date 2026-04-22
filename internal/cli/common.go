package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"architex/baseline"
	"architex/config"
	"architex/graph"
	"architex/models"
	"architex/parser"
	"architex/risk"
)

// buildBaseHeadWith parses both directories and returns the constructed
// graphs. The same cfg is applied to both base and head graphs; this matches
// the user expectation that "ignore path X" means "X never enters my graph",
// regardless of which side. On error it writes a stderr diagnostic and
// returns a non-zero exit code so the caller can early-return.
func buildBaseHeadWith(baseDir, headDir string, cfg *config.Config) (models.Graph, models.Graph, int) {
	base, err := buildGraphWith(baseDir, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[architex] error parsing base: %v\n", err)
		return models.Graph{}, models.Graph{}, 1
	}
	head, err := buildGraphWith(headDir, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[architex] error parsing head: %v\n", err)
		return models.Graph{}, models.Graph{}, 1
	}
	return base, head, 0
}

// buildGraphWith parses a directory and constructs the graph, emitting
// parser warnings to stderr as a side effect. A non-nil cfg is forwarded
// to the parser so ignore.paths patterns drop matching .tf files before
// they ever enter the graph.
func buildGraphWith(dir string, cfg *config.Config) (models.Graph, error) {
	resources, warnings, err := parser.ParseDirWith(dir, cfg)
	if err != nil {
		return models.Graph{}, err
	}
	for _, w := range warnings {
		fmt.Fprintf(os.Stderr, "[architex] WARN [%s]: %s\n", w.Category, w.Message)
	}
	return graph.Build(resources, warnings), nil
}

// loadConfigForDir loads `.architex.yml` from dir (no error if absent), then
// merges any inline `# architex:ignore=...` directives discovered in the
// same directory tree. A returned nil *Config means "no config" -- callers
// must behave identically to a v1.1 run in that case.
//
// Errors are emitted to stderr but never abort the run; a malformed config
// file degrades to "no config" so a typo cannot brick CI.
func loadConfigForDir(dir string) *config.Config {
	cfgPath := filepath.Join(dir, config.FileName)
	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[architex] WARN config: %v -- continuing with defaults\n", err)
		return nil
	}
	inline, err := config.ScanInlineSuppressions(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[architex] WARN config: inline scan: %v\n", err)
	}
	if cfg == nil && len(inline) == 0 {
		return nil
	}
	if cfg == nil {
		cfg = &config.Config{}
	}
	cfg.Add(inline...)
	fmt.Fprintf(os.Stderr, "[architex] config: loaded (%d rule overrides, %d ignore paths, %d suppressions)\n",
		len(cfg.Rules), len(cfg.Ignore.Paths), len(cfg.Suppressions))
	return cfg
}

// loadBaselineForDir loads `.architex/baseline.json` from `dir` (no error
// if absent), returning nil when no baseline is present so the caller's
// `first_time_*` rules stay silent. A malformed baseline is treated as
// "no baseline" with a stderr warning, matching loadConfigForDir's
// degradation policy: a typo in baseline.json must never brick CI.
func loadBaselineForDir(dir string) *baseline.Baseline {
	path := filepath.Join(dir, baseline.FileName)
	bl, err := baseline.Load(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[architex] WARN baseline: %v -- continuing without baseline rules\n", err)
		return nil
	}
	if bl == nil {
		return nil
	}
	fmt.Fprintf(os.Stderr,
		"[architex] baseline: loaded %s (%d provider types, %d abstract types, %d edge pairs)\n",
		path, len(bl.ProviderTypes), len(bl.AbstractTypes), len(bl.EdgePairs))
	return bl
}

// logRisk prints a one-line summary of the risk verdict to stderr followed
// by one line per fired rule. Stdout is reserved for the machine-readable
// artifact, so this is the human-readable channel only.
func logRisk(r risk.RiskResult) {
	fmt.Fprintf(os.Stderr, "[architex] risk level: %.1f/10 (higher = more risk) | severity: %s | status: %s\n",
		r.Score, r.Severity, r.Status)
	for _, reason := range r.Reasons {
		fmt.Fprintf(os.Stderr, "[architex]   [%.1f] %s -- %s\n", reason.Weight, reason.RuleID, reason.Message)
	}
}

// splitFlagsAndPositional separates an arg slice into flag args (anything
// starting with "-" plus its value when not "=" separated) and positional
// args. This lets users put flags before OR after positionals on the CLI,
// which is friendlier than Go's default flag-parser behavior.
//
// Recognized as a flag value-pair when the flag does not contain "=" and
// the next arg does not start with "-".
func splitFlagsAndPositional(args []string) (flags []string, positional []string) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			positional = append(positional, args[i+1:]...)
			return
		case len(a) > 1 && a[0] == '-':
			flags = append(flags, a)
			if !containsByte(a, '=') && i+1 < len(args) {
				next := args[i+1]
				if len(next) == 0 || next[0] != '-' {
					flags = append(flags, next)
					i++
				}
			}
		default:
			positional = append(positional, a)
		}
	}
	return
}

func containsByte(s string, c byte) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return true
		}
	}
	return false
}

// writeJSON encodes v as indented JSON to w. On encode error it writes a
// diagnostic to stderr and returns a non-zero exit code; callers propagate
// the int verbatim.
func writeJSON(w io.Writer, v any) int {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		fmt.Fprintf(os.Stderr, "[architex] error encoding JSON: %v\n", err)
		return 1
	}
	return 0
}
