// architex is a DevSecOps CLI that parses Terraform, builds an architecture
// graph, computes deltas between graph versions, and evaluates architectural
// risk. Subcommands:
//
//	architex graph    <dir>                 -- build and print the graph for one dir
//	architex diff     <base-dir> <head-dir> -- print the semantic delta between two graphs
//	architex score    <base-dir> <head-dir> -- print the risk evaluation of the delta
//	architex report   <base-dir> <head-dir> -- print the full PR-ready Markdown payload
//	architex sanitize <base-dir> <head-dir> -- print the sanitized egress payload (JSON)
//
// Stdout: machine-readable artifact (JSON for graph/diff/score/sanitize, Markdown for report).
// Stderr: human-readable progress + warnings, prefixed with "[architex]".
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"architex/delta"
	"architex/graph"
	"architex/interpreter"
	"architex/models"
	"architex/parser"
	"architex/risk"
)

const usage = `architex -- DevSecOps architecture graph + risk CLI

Usage:
  architex graph    <dir>
  architex diff     <base-dir> <head-dir>
  architex score    <base-dir> <head-dir>
  architex report   <base-dir> <head-dir> [--out <dir>] [--salt <salt>]
  architex sanitize <base-dir> <head-dir> [--salt <salt>]
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}

	switch os.Args[1] {
	case "graph":
		os.Exit(runGraph(os.Args[2:]))
	case "diff":
		os.Exit(runDiff(os.Args[2:]))
	case "score":
		os.Exit(runScore(os.Args[2:]))
	case "report":
		os.Exit(runReport(os.Args[2:]))
	case "sanitize":
		os.Exit(runSanitize(os.Args[2:]))
	case "-h", "--help", "help":
		fmt.Fprint(os.Stdout, usage)
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand %q\n\n%s", os.Args[1], usage)
		os.Exit(2)
	}
}

func runGraph(args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: architex graph <dir>")
		return 2
	}
	g, err := buildGraph(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "[architex] error: %v\n", err)
		return 1
	}
	return writeJSON(os.Stdout, g)
}

func runDiff(args []string) int {
	if len(args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: architex diff <base-dir> <head-dir>")
		return 2
	}
	base, head, code := buildBaseHead(args[0], args[1])
	if code != 0 {
		return code
	}
	d := delta.Compare(base, head)
	fmt.Fprintf(os.Stderr, "[architex] delta: %s\n", delta.HumanSummary(d))
	return writeJSON(os.Stdout, d)
}

func runScore(args []string) int {
	if len(args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: architex score <base-dir> <head-dir>")
		return 2
	}
	base, head, code := buildBaseHead(args[0], args[1])
	if code != 0 {
		return code
	}
	d := delta.Compare(base, head)
	result := risk.Evaluate(d)

	fmt.Fprintf(os.Stderr, "[architex] delta: %s\n", delta.HumanSummary(d))
	logRisk(result)
	return writeJSON(os.Stdout, result)
}

func runReport(args []string) int {
	fs := flag.NewFlagSet("report", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	out := fs.String("out", "", "directory to write audit bundle into (timestamped subdir)")
	salt := fs.String("salt", "", "salt for sanitization hashing in audit egress.json")
	flagArgs, positional := splitFlagsAndPositional(args)
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	rest := positional
	if len(rest) != 2 {
		fmt.Fprintln(os.Stderr, "usage: architex report <base-dir> <head-dir> [--out <dir>] [--salt <salt>]")
		return 2
	}

	base, head, code := buildBaseHead(rest[0], rest[1])
	if code != 0 {
		return code
	}
	d := delta.Compare(base, head)
	result := risk.Evaluate(d)
	rep := interpreter.Render(d, result, nil)

	logRisk(result)

	if *out != "" {
		bundle, err := interpreter.WriteAudit(rep, interpreter.AuditOptions{
			OutDir:   *out,
			BaseDir:  rest[0],
			HeadDir:  rest[1],
			HashSalt: *salt,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "[architex] audit error: %v\n", err)
			return 1
		}
		fmt.Fprintf(os.Stderr, "[architex] audit bundle: %s\n", bundle.Path)
	}

	if _, err := io.WriteString(os.Stdout, interpreter.FormatMarkdown(rep)); err != nil {
		fmt.Fprintf(os.Stderr, "[architex] write error: %v\n", err)
		return 1
	}
	return 0
}

func runSanitize(args []string) int {
	fs := flag.NewFlagSet("sanitize", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	salt := fs.String("salt", "", "salt for sanitization hashing")
	flagArgs, positional := splitFlagsAndPositional(args)
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	rest := positional
	if len(rest) != 2 {
		fmt.Fprintln(os.Stderr, "usage: architex sanitize <base-dir> <head-dir> [--salt <salt>]")
		return 2
	}

	base, head, code := buildBaseHead(rest[0], rest[1])
	if code != 0 {
		return code
	}
	d := delta.Compare(base, head)
	result := risk.Evaluate(d)
	rep := interpreter.Render(d, result, nil)
	payload := interpreter.Sanitize(rep, interpreter.SanitizationPolicy{HashSalt: *salt})

	fmt.Fprintf(os.Stderr, "[architex] egress payload schema=%s severity=%s status=%s\n",
		payload.SchemaVersion, payload.Severity, payload.Status)
	return writeJSON(os.Stdout, payload)
}

// buildBaseHead parses both directories and returns the constructed graphs.
// On error it writes a stderr diagnostic and returns a non-zero exit code.
func buildBaseHead(baseDir, headDir string) (models.Graph, models.Graph, int) {
	base, err := buildGraph(baseDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[architex] error parsing base: %v\n", err)
		return models.Graph{}, models.Graph{}, 1
	}
	head, err := buildGraph(headDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[architex] error parsing head: %v\n", err)
		return models.Graph{}, models.Graph{}, 1
	}
	return base, head, 0
}

func logRisk(r risk.RiskResult) {
	fmt.Fprintf(os.Stderr, "[architex] risk level: %.1f/10 (higher = more risk) | severity: %s | status: %s\n",
		r.Score, r.Severity, r.Status)
	for _, reason := range r.Reasons {
		fmt.Fprintf(os.Stderr, "[architex]   [%.1f] %s -- %s\n", reason.Weight, reason.RuleID, reason.Message)
	}
}

// buildGraph parses a directory and constructs the graph, emitting parser
// warnings to stderr as a side effect.
func buildGraph(dir string) (models.Graph, error) {
	resources, warnings, err := parser.ParseDir(dir)
	if err != nil {
		return models.Graph{}, err
	}
	for _, w := range warnings {
		fmt.Fprintf(os.Stderr, "[architex] WARN [%s]: %s\n", w.Category, w.Message)
	}
	return graph.Build(resources, warnings), nil
}

// splitFlagsAndPositional separates an arg slice into flag args (anything
// starting with "-" plus its value when not "=" separated) and positional
// args. This lets users put flags before OR after positionals on the CLI,
// which is friendlier than Go's default flag-parser behavior.
//
// Recognized as a flag value-pair when the flag does not contain "=" and the
// next arg does not start with "-".
func splitFlagsAndPositional(args []string) (flags []string, positional []string) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			positional = append(positional, args[i+1:]...)
			return
		case len(a) > 1 && a[0] == '-':
			flags = append(flags, a)
			if !contains(a, '=') && i+1 < len(args) {
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

func contains(s string, c byte) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return true
		}
	}
	return false
}

func writeJSON(w io.Writer, v any) int {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		fmt.Fprintf(os.Stderr, "[architex] error encoding JSON: %v\n", err)
		return 1
	}
	return 0
}
