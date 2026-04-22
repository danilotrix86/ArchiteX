package cli

import (
	"flag"
	"fmt"
	"os"
	"time"

	"architex/delta"
	"architex/interpreter"
	"architex/risk"
)

// RunSanitize implements `architex sanitize <base-dir> <head-dir>`: runs the
// full pipeline but emits ONLY the sanitized egress payload (the schema
// documented in docs/egress-schema.json). Use this to inspect what would
// leave the runner before wiring up an actual upload.
func RunSanitize(args []string) int {
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

	cfg := loadConfigForDir(rest[1])
	bl := loadBaselineForDir(rest[1])
	base, head, code := buildBaseHeadWith(rest[0], rest[1], cfg)
	if code != 0 {
		return code
	}
	d := delta.Compare(base, head)
	result := risk.EvaluateWithBaseline(d, cfg, bl, time.Now())
	rep := interpreter.RenderWithGraph(d, result, head, nil)
	payload := interpreter.Sanitize(rep, interpreter.SanitizationPolicy{HashSalt: *salt})

	fmt.Fprintf(os.Stderr, "[architex] egress payload schema=%s severity=%s status=%s\n",
		payload.SchemaVersion, payload.Severity, payload.Status)
	return writeJSON(os.Stdout, payload)
}
