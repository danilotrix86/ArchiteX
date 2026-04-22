package cli

import (
	"fmt"
	"os"
)

// RunGraph implements `architex graph <dir>`: parse one directory, build
// the architecture graph, and print it as indented JSON on stdout.
func RunGraph(args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: architex graph <dir>")
		return 2
	}
	cfg := loadConfigForDir(args[0])
	g, err := buildGraphWith(args[0], cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[architex] error: %v\n", err)
		return 1
	}
	return writeJSON(os.Stdout, g)
}
