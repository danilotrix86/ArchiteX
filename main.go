// architex is a DevSecOps CLI that parses Terraform, builds an architecture
// graph, computes deltas between graph versions, and evaluates architectural
// risk. Subcommands:
//
//	architex graph <dir>                 -- build and print the graph for one dir
//	architex diff  <base-dir> <head-dir> -- print the semantic delta between two graphs
//	architex score <base-dir> <head-dir> -- print the risk evaluation of the delta
//
// JSON goes to stdout. Warnings and human-readable summaries go to stderr,
// prefixed with "[architex]".
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"architex/delta"
	"architex/graph"
	"architex/models"
	"architex/parser"
	"architex/risk"
)

const usage = `architex -- DevSecOps architecture graph + risk CLI

Usage:
  architex graph <dir>
  architex diff  <base-dir> <head-dir>
  architex score <base-dir> <head-dir>
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
	base, err := buildGraph(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "[architex] error parsing base: %v\n", err)
		return 1
	}
	head, err := buildGraph(args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "[architex] error parsing head: %v\n", err)
		return 1
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
	base, err := buildGraph(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "[architex] error parsing base: %v\n", err)
		return 1
	}
	head, err := buildGraph(args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "[architex] error parsing head: %v\n", err)
		return 1
	}
	d := delta.Compare(base, head)
	result := risk.Evaluate(d)

	fmt.Fprintf(os.Stderr, "[architex] delta: %s\n", delta.HumanSummary(d))
	fmt.Fprintf(os.Stderr, "[architex] score: %.1f | severity: %s | status: %s\n",
		result.Score, result.Severity, result.Status)
	for _, r := range result.Reasons {
		fmt.Fprintf(os.Stderr, "[architex]   [%.1f] %s -- %s\n", r.Weight, r.RuleID, r.Message)
	}
	return writeJSON(os.Stdout, result)
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

func writeJSON(w io.Writer, v any) int {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		fmt.Fprintf(os.Stderr, "[architex] error encoding JSON: %v\n", err)
		return 1
	}
	return 0
}
