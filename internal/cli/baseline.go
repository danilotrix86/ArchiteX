package cli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"architex/baseline"
)

// RunBaseline writes (or extends) a `.architex/baseline.json` snapshot of a
// directory's current architecture graph. Used to enable the Phase 7 PR5
// `first_time_*` anomaly rules. Without `--merge` the file is replaced by
// the current snapshot; with `--merge` the existing file is loaded and the
// current snapshot is unioned into it (so a sweep across multiple stacks
// can incrementally build a single baseline). The output path defaults to
// `<dir>/.architex/baseline.json`.
func RunBaseline(args []string) int {
	fs := flag.NewFlagSet("baseline", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	out := fs.String("out", "", "explicit output path (default: <dir>/.architex/baseline.json)")
	merge := fs.Bool("merge", false, "union with the existing baseline at --out instead of replacing it")
	flagArgs, positional := splitFlagsAndPositional(args)
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	if len(positional) != 1 {
		fmt.Fprintln(os.Stderr, "usage: architex baseline <dir> [--out <path>] [--merge]")
		return 2
	}
	dir := positional[0]
	outPath := *out
	if outPath == "" {
		outPath = filepath.Join(dir, baseline.FileName)
	}

	cfg := loadConfigForDir(dir)
	g, err := buildGraphWith(dir, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[architex] error: %v\n", err)
		return 1
	}

	snap := baseline.FromGraph(g, time.Now())
	if *merge {
		existing, err := baseline.Load(outPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[architex] WARN baseline merge: %v -- writing fresh snapshot\n", err)
		} else if existing != nil {
			snap = baseline.Merge(existing, snap)
			snap.GeneratedAt = time.Now().UTC()
		}
	}

	if err := baseline.Save(outPath, snap); err != nil {
		fmt.Fprintf(os.Stderr, "[architex] error: %v\n", err)
		return 1
	}
	fmt.Fprintf(os.Stderr,
		"[architex] baseline written: %s (%d provider types, %d abstract types, %d edge pairs)\n",
		outPath, len(snap.ProviderTypes), len(snap.AbstractTypes), len(snap.EdgePairs))
	return 0
}
