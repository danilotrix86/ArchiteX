package cli

import (
	"fmt"
	"os"

	"architex/delta"
)

// RunDiff implements `architex diff <base-dir> <head-dir>`: compute the
// semantic delta between two graphs and print it as indented JSON on
// stdout. The human summary goes to stderr.
func RunDiff(args []string) int {
	if len(args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: architex diff <base-dir> <head-dir>")
		return 2
	}
	cfg := loadConfigForDir(args[1])
	base, head, code := buildBaseHeadWith(args[0], args[1], cfg)
	if code != 0 {
		return code
	}
	d := delta.Compare(base, head)
	fmt.Fprintf(os.Stderr, "[architex] delta: %s\n", delta.HumanSummary(d))
	return writeJSON(os.Stdout, d)
}
