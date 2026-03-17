package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/git-pkgs/brief"
	"github.com/git-pkgs/brief/detect"
	"github.com/git-pkgs/brief/kb"
	"github.com/git-pkgs/brief/report"
)

func cmdMissing(args []string) {
	fs := flag.NewFlagSet("brief missing", flag.ExitOnError)
	jsonFlag := fs.Bool("json", false, "Force JSON output")
	humanFlag := fs.Bool("human", false, "Force human-readable output")
	scanDepth := fs.Int("scan-depth", 0, "Max directory depth for language detection (default 4)")
	skip := fs.String("skip", "", "Additional directories to skip, comma-separated")
	_ = fs.Parse(args)

	path := "."
	if fs.NArg() > 0 {
		path = fs.Arg(0)
	}

	knowledgeBase, err := kb.Load(brief.KnowledgeFS)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error loading knowledge base: %v\n", err)
		os.Exit(1)
	}

	engine := detect.New(knowledgeBase, path)
	engine.ScanDepth = *scanDepth
	if *skip != "" {
		engine.SkipDirs = strings.Split(*skip, ",")
	}
	r, err := engine.Run()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	mr := engine.Missing(r)

	useJSON := *jsonFlag || (!*humanFlag && !isTTY())

	if useJSON {
		if err := report.MissingJSON(os.Stdout, mr); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "error writing JSON: %v\n", err)
			os.Exit(1)
		}
	} else {
		report.MissingHuman(os.Stdout, mr)
	}
}
