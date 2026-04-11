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

// outputFormat is the resolved output mode after flag parsing.
type outputFormat int

const (
	formatJSON outputFormat = iota
	formatHuman
	formatMarkdown
)

// runDetection handles the common preamble for subcommands that run detection
// and post-process the report: standard flags, KB load, engine setup, run.
// Exits on error. Returns the engine (for post-processors), the report,
// and the resolved output format.
func runDetection(name string, args []string) (*detect.Engine, *brief.Report, outputFormat) {
	fs := flag.NewFlagSet("brief "+name, flag.ExitOnError)
	jsonFlag := fs.Bool("json", false, "Force JSON output")
	humanFlag := fs.Bool("human", false, "Force human-readable output")
	markdownFlag := fs.Bool("markdown", false, "Force markdown output")
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

	format := formatHuman
	switch {
	case *markdownFlag:
		format = formatMarkdown
	case *jsonFlag || (!*humanFlag && !isTTY()):
		format = formatJSON
	}

	return engine, r, format
}

func writeJSONOrExit(err error) {
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error writing JSON: %v\n", err)
		os.Exit(1)
	}
}

func cmdThreatModel(args []string) {
	engine, r, format := runDetection("threat-model", args)
	tr := engine.ThreatModel(r)

	switch format {
	case formatMarkdown:
		report.ThreatMarkdown(os.Stdout, tr)
	case formatJSON:
		writeJSONOrExit(report.ThreatJSON(os.Stdout, tr))
	default:
		report.ThreatHuman(os.Stdout, tr)
	}
}

func cmdSinks(args []string) {
	engine, r, format := runDetection("sinks", args)
	sr := engine.Sinks(r)

	switch format {
	case formatMarkdown:
		report.SinksMarkdown(os.Stdout, sr)
	case formatJSON:
		writeJSONOrExit(report.SinksJSON(os.Stdout, sr))
	default:
		report.SinksHuman(os.Stdout, sr)
	}
}
