package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/git-pkgs/brief"
	"github.com/git-pkgs/brief/detect"
	"github.com/git-pkgs/brief/kb"
	"github.com/git-pkgs/brief/report"

	"golang.org/x/term"
)

func main() {
	jsonFlag := flag.Bool("json", false, "Force JSON output")
	humanFlag := flag.Bool("human", false, "Force human-readable output")
	verbose := flag.Bool("verbose", false, "Include breadcrumb/reference information")
	version := flag.Bool("version", false, "Print version and exit")
	flag.Parse()

	if *version {
		fmt.Println("brief", brief.Version)
		os.Exit(0)
	}

	path := "."
	if flag.NArg() > 0 {
		path = flag.Arg(0)
	}

	knowledgeBase, err := kb.Load(brief.KnowledgeFS)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading knowledge base: %v\n", err)
		os.Exit(1)
	}

	engine := detect.New(knowledgeBase, path)
	r, err := engine.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	useJSON := *jsonFlag || (!*humanFlag && !isTTY())

	if useJSON {
		if err := report.JSON(os.Stdout, r); err != nil {
			fmt.Fprintf(os.Stderr, "error writing JSON: %v\n", err)
			os.Exit(1)
		}
	} else {
		report.Human(os.Stdout, r, *verbose)
	}
}

func isTTY() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}
