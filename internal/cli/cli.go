// Package cli implements every architex subcommand. It exists to keep
// cmd/architex/main.go a thin dispatcher (just `version` flag handling +
// a call to Run); every command's flags, helpers, and exit-code policy
// live here so that subcommand changes don't ripple into the binary's
// entrypoint.
//
// Stdout / stderr discipline matches the package-level docs that used to
// live in main.go:
//
//   - Stdout: machine-readable artifact (JSON for graph/diff/score/sanitize,
//     Markdown for report).
//   - Stderr: human-readable progress + warnings, prefixed with "[architex]".
//
// The `comment` subcommand is the ONLY architex command that performs
// outbound network I/O. The analysis pipeline (parser/graph/delta/risk/
// interpreter) never touches the network.
package cli

import (
	"fmt"
	"os"
)

// Usage is the canonical CLI usage string. cmd/architex/main.go re-exports
// it indirectly via Run; tests pin it as a stable surface.
const Usage = `architex -- DevSecOps architecture graph + risk CLI

Usage:
  architex graph    <dir>
  architex diff     <base-dir> <head-dir>
  architex score    <base-dir> <head-dir>
  architex report   <base-dir> <head-dir> [--out <dir>] [--salt <salt>]
  architex sanitize <base-dir> <head-dir> [--salt <salt>]
  architex baseline <dir> [--out <path>] [--merge]
  architex comment  <bundle-dir> --repo <owner/repo> --pr <num> [--mode advisory|blocking] [--token-env GITHUB_TOKEN]
`

// Run dispatches to the requested subcommand. argv is the post-binary slice
// (i.e. os.Args[1:]). The returned int is the process exit code; the caller
// is expected to os.Exit() with it. Run never calls os.Exit itself so that
// tests can drive it.
//
// `version` and friends are handled by cmd/architex/main.go BEFORE Run is
// called -- those values are populated via ldflags into the main package
// and Run has no way to see them.
func Run(argv []string) int {
	if len(argv) < 1 {
		fmt.Fprint(os.Stderr, Usage)
		return 2
	}

	switch argv[0] {
	case "graph":
		return RunGraph(argv[1:])
	case "diff":
		return RunDiff(argv[1:])
	case "score":
		return RunScore(argv[1:])
	case "report":
		return RunReport(argv[1:])
	case "sanitize":
		return RunSanitize(argv[1:])
	case "baseline":
		return RunBaseline(argv[1:])
	case "comment":
		return RunComment(argv[1:])
	case "-h", "--help", "help":
		fmt.Fprint(os.Stdout, Usage)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand %q\n\n%s", argv[0], Usage)
		return 2
	}
}
