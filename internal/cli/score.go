package cli

import (
	"fmt"
	"os"
	"time"

	"architex/delta"
	"architex/risk"
)

// RunScore implements `architex score <base-dir> <head-dir>`: compute the
// delta and run the risk engine on it, printing the RiskResult as JSON on
// stdout. The verdict + per-rule breakdown goes to stderr via logRisk.
func RunScore(args []string) int {
	if len(args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: architex score <base-dir> <head-dir>")
		return 2
	}
	cfg := loadConfigForDir(args[1])
	bl := loadBaselineForDir(args[1])
	base, head, code := buildBaseHeadWith(args[0], args[1], cfg)
	if code != 0 {
		return code
	}
	d := delta.Compare(base, head)
	result := risk.EvaluateWithBaseline(d, cfg, bl, time.Now())

	fmt.Fprintf(os.Stderr, "[architex] delta: %s\n", delta.HumanSummary(d))
	logRisk(result)
	return writeJSON(os.Stdout, result)
}
