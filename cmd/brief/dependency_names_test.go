package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/git-pkgs/brief"
)

func TestScanPyPIDependencyNames(t *testing.T) {
	const helperEnv = "BRIEF_PYPI_NAMES_ROOT"
	if root := os.Getenv(helperEnv); root != "" {
		cmdScan([]string{"-json", root})
		return
	}
	root, err := filepath.Abs("../../testdata/pypi-names-project")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestScanPyPIDependencyNames$")
	cmd.Env = append(os.Environ(), helperEnv+"="+root, "PATH=")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}
	var report brief.Report
	if err := json.Unmarshal(out, &report); err != nil {
		t.Fatalf("parsing scan output: %v\n%s", err, out)
	}
	if !reportHasTool(&report, "build", "Flask") || !reportHasTool(&report, "docs", "MyST-Parser") {
		t.Fatalf("tools = %+v, want Flask and MyST-Parser", report.Tools)
	}
}

func TestDiffPyPIDependencyNames(t *testing.T) {
	const helperEnv = "BRIEF_DIFF_PYPI_NAMES"
	if os.Getenv(helperEnv) != "" {
		cmdDiff([]string{"-json", "HEAD"})
		os.Exit(0)
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := t.TempDir()
	initGitScanFixture(t, root)
	writeScanFixture(t, root, "requirements.txt", "FLask\n")
	runGitFixture(t, root, "add", "requirements.txt")
	runGitFixture(t, root, "commit", "-q", "-m", "initial")
	writeScanFixture(t, root, "requirements.txt", "FLask\nMYST_PARSER\n")
	cmd := exec.Command(os.Args[0], "-test.run=^TestDiffPyPIDependencyNames$")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), helperEnv+"=1")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("diff failed: %v", err)
	}
	var report brief.Report
	if err := json.Unmarshal(out, &report); err != nil {
		t.Fatalf("parsing diff output: %v\n%s", err, out)
	}
	if !reportHasTool(&report, "build", "Flask") || !reportHasTool(&report, "docs", "MyST-Parser") {
		t.Fatalf("tools = %+v, want Flask and MyST-Parser", report.Tools)
	}
}
