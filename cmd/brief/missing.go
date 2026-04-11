package main

import (
	"os"

	"github.com/git-pkgs/brief/report"
)

func cmdMissing(args []string) {
	engine, r, format := runDetection("missing", args)
	mr := engine.Missing(r)

	switch format {
	case formatMarkdown:
		report.MissingMarkdown(os.Stdout, mr)
	case formatJSON:
		writeJSONOrExit(report.MissingJSON(os.Stdout, mr))
	default:
		report.MissingHuman(os.Stdout, mr)
	}
}
