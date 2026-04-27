package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"runtime/debug"
	"strings"

	"github.com/git-pkgs/brief/remote"
	"github.com/git-pkgs/outline"
)

func cmdOutline(args []string) {
	// Re-enable GC: outline parses every source file via tree-sitter, which
	// allocates substantial arena memory per file. Without GC the pooled
	// arenas accumulate unboundedly across thousands of files.
	const defaultGCPercent = 100
	debug.SetGCPercent(defaultGCPercent)

	fs := flag.NewFlagSet("brief outline", flag.ExitOnError)
	xmlFlag := fs.Bool("xml", false, "Output XML instead of markdown")
	full := fs.Bool("full", false, "Include full file contents (no body stripping)")
	ignore := fs.String("ignore", "", "Additional ignore patterns, comma-separated")
	maxFileSize := fs.Int64("max-file-size", 0, "Skip files larger than this many bytes (default 1MB)")
	maxFiles := fs.Int("max-files", 0, "Stop after this many files (default 10000)")
	keep := fs.Bool("keep", false, "Keep downloaded remote source")
	depth := fs.Int("depth", -1, "Git clone depth (0 = full clone, default shallow)")
	dir := fs.String("dir", "", "Directory to clone remote source into")
	_ = fs.Parse(args)

	path := "."
	if fs.NArg() > 0 {
		path = fs.Arg(0)
	}

	src, err := remote.Resolve(context.Background(), path, remote.Options{
		Keep:  *keep,
		Depth: *depth,
		Dir:   *dir,
	})
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	opts := outline.Options{
		Compress:    !*full,
		MaxFileSize: *maxFileSize,
		MaxFiles:    *maxFiles,
	}
	if *ignore != "" {
		opts.Ignore = strings.Split(*ignore, ",")
	}

	code := runOutline(src.Dir, opts, *xmlFlag)
	src.Cleanup()
	os.Exit(code)
}

func runOutline(dir string, opts outline.Options, xmlOut bool) int {
	r, err := outline.Pack(dir, opts)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	if xmlOut {
		err = r.XML(os.Stdout)
	} else {
		err = r.Markdown(os.Stdout)
	}
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error writing output: %v\n", err)
		return 1
	}
	return 0
}
