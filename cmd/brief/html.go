package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/git-pkgs/brief"
	"github.com/git-pkgs/brief/detect"
	"github.com/git-pkgs/brief/kb"
	"github.com/git-pkgs/brief/remote"
	"github.com/git-pkgs/brief/report"
)

func cmdHTML(args []string) {
	fs := flag.NewFlagSet("brief html", flag.ExitOnError)
	out := fs.String("o", "", "Output file (default stdout)")
	keep := fs.Bool("keep", false, "Keep downloaded remote source")
	depth := fs.Int("depth", -1, "Git clone depth (0 = full clone, default shallow)")
	dir := fs.String("dir", "", "Directory to clone remote source into")
	scanDepth := fs.Int("scan-depth", 0, "Max directory depth for language detection (0 = unlimited)")
	skip := fs.String("skip", "", "Additional directories to skip, comma-separated")
	tracked := fs.Bool("tracked", false, "Only consider files tracked by git")
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

	code := runHTML(src.Dir, *out, *scanDepth, *skip, *tracked)
	src.Cleanup()
	os.Exit(code)
}

func runHTML(dir, out string, scanDepth int, skip string, tracked bool) int {
	knowledgeBase, err := kb.Load(brief.KnowledgeFS)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error loading knowledge base: %v\n", err)
		return 1
	}

	engine := detect.New(knowledgeBase, dir)
	engine.ScanDepth = scanDepth
	engine.TrackedOnly = tracked
	if skip != "" {
		engine.SkipDirs = strings.Split(skip, ",")
	}
	r, err := engine.Run()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	var w io.Writer = os.Stdout
	if out != "" {
		f, ferr := os.Create(out) //nolint:gosec
		if ferr != nil {
			_, _ = fmt.Fprintf(os.Stderr, "error creating %s: %v\n", out, ferr)
			return 1
		}
		defer func() { _ = f.Close() }()
		w = f
	}

	if err := report.HTML(w, r); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error rendering HTML: %v\n", err)
		return 1
	}
	if out != "" {
		_, _ = fmt.Fprintf(os.Stderr, "wrote %s\n", out)
	}
	return 0
}
