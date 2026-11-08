package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"testing"

	"github.com/git-pkgs/brief"
)

func TestScanNotebookFlag(t *testing.T) {
	const helper = "BRIEF_NOTEBOOK_HELPER"
	if root := os.Getenv(helper); root != "" {
		cmdScan([]string{"-json", "-notebooks", root})
		return
	}
	root := t.TempDir()
	fixture, err := os.ReadFile("../../testdata/notebook-project/analysis.ipynb")
	if err != nil {
		t.Fatal(err)
	}
	writeScanFixture(t, root, "analysis.ipynb", string(fixture))
	cmd := exec.Command(os.Args[0], "-test.run=^TestScanNotebookFlag$")
	cmd.Env = append(os.Environ(), helper+"="+root, "PATH=")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("scan command failed: %v", err)
	}
	var report brief.Report
	if err := json.Unmarshal(out, &report); err != nil {
		t.Fatal(err)
	}
	if report.Notebooks == nil || len(report.Notebooks.Files) != 1 || report.Notebooks.Files[0].Language != "julia" || report.Notebooks.Files[0].CodeLines != 1 {
		t.Fatalf("unexpected CLI notebook statistics: %+v", report.Notebooks)
	}
}
