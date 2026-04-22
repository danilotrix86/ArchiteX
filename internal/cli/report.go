package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"architex/delta"
	"architex/interpreter"
	"architex/risk"
)

// RunReport implements `architex report <base-dir> <head-dir>`: the full
// pipeline (parse + diff + score + render). With `--out <dir>` it also
// writes a timestamped audit bundle (summary.md, score.json, egress.json,
// report.html, manifest.json + diagram.mmd) into a subdirectory of
// `<dir>`. Stdout always carries the rendered Markdown.
func RunReport(args []string) int {
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

	cfg := loadConfigForDir(rest[1])
	bl := loadBaselineForDir(rest[1])
	base, head, code := buildBaseHeadWith(rest[0], rest[1], cfg)
	if code != 0 {
		return code
	}
	d := delta.Compare(base, head)
	result := risk.EvaluateWithBaseline(d, cfg, bl, time.Now())
	rep := interpreter.RenderWithGraph(d, result, head, nil)

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
