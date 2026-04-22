// architex is a DevSecOps CLI that parses Terraform, builds an architecture
// graph, computes deltas between graph versions, and evaluates architectural
// risk. Subcommands:
//
//	architex graph    <dir>                 -- build and print the graph for one dir
//	architex diff     <base-dir> <head-dir> -- print the semantic delta between two graphs
//	architex score    <base-dir> <head-dir> -- print the risk evaluation of the delta
//	architex report   <base-dir> <head-dir> -- print the full PR-ready Markdown payload
//	architex sanitize <base-dir> <head-dir> -- print the sanitized egress payload (JSON)
//	architex baseline <dir>                 -- snapshot the graph for first-time-* anomaly rules
//	architex comment  <bundle-dir>          -- upsert the bundle's summary.md as a sticky PR comment
//
// Stdout: machine-readable artifact (JSON for graph/diff/score/sanitize, Markdown for report).
// Stderr: human-readable progress + warnings, prefixed with "[architex]".
//
// The `comment` subcommand is the ONLY architex command that performs
// outbound network I/O. The analysis pipeline (parser/graph/delta/risk/
// interpreter) never touches the network.
//
// All subcommand logic lives in architex/internal/cli; this file is a thin
// dispatcher whose only special-case is `version`, which reads ldflag-
// injected build metadata that can't escape the main package.
package main

import (
	"fmt"
	"os"

	"architex/internal/cli"

	// Blank-import the rules aggregator so every migrated risk rule
	// registers with architex/risk/api before main() runs. The risk
	// package itself does NOT import this aggregator -- the binary
	// (and the golden-snapshot test harness) is the wiring point that
	// closes the dependency triangle without creating an import cycle.
	_ "architex/risk/rules"
)

// version, commit, and date are populated at build time by goreleaser
// (see .goreleaser.yaml). For `go build` / `go install` callers (and for
// `go test`) they keep their dev-time defaults. They MUST live in package
// main because `-ldflags -X main.version=...` resolves only here.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	if len(os.Args) >= 2 {
		switch os.Args[1] {
		case "-v", "--version", "version":
			fmt.Fprintf(os.Stdout, "architex %s (commit %s, built %s)\n", version, commit, date)
			os.Exit(0)
		}
	}
	os.Exit(cli.Run(os.Args[1:]))
}
