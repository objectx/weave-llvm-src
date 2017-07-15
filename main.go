package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

var (
	// ProgramName holds the name of current executing binary.
	ProgramName = getExecutableName()
	option      struct {
		verbose bool
	}
)

func main() {
	flag.Usage = func() {
		base := filepath.Base(ProgramName)
		fmt.Fprintf(os.Stderr, `%s Constructs LLVM source tree from downloaded archives.
Usage: %s [options] <src-dir> [<dst-dir>]

Available options:
`, base, ProgramName)
		flag.PrintDefaults()
		os.Exit(1)
	}
	flag.BoolVar(&option.verbose, "v", false, "Be verbose")
	flag.Parse()
	srcDir := "."
	dstDir := "."
	switch flag.NArg() {
	case 0:
		/* NO-OP */
	case 1:
		srcDir = flag.Arg(0)
	case 2:
		fallthrough
	default:
		srcDir = flag.Arg(0)
		dstDir = flag.Arg(1)
	}

	if err := weave(dstDir, srcDir); err != nil {
		fmt.Fprintf(os.Stderr, "%s:error: %v\n", ProgramName, err)
		os.Exit(1)
	}
	os.Exit(0)
}

// getExecutableName obtains the executable name.
func getExecutableName() string {
	if p, err := os.Executable(); err == nil {
		return p
	}
	return "weave-llvm-src"
}

// verbose Show verbose message.
func verbose(format string, args ...interface{}) {
	if option.verbose {
		fmt.Fprintf(os.Stderr, "%s: ", ProgramName)
		fmt.Fprintf(os.Stderr, format, args...)
		fmt.Fprintln(os.Stderr)
	}
}
