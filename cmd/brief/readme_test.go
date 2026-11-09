package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"testing"

	"github.com/git-pkgs/brief"
)

func TestScanReadmeSelection(t *testing.T) {
	const helperEnv = "BRIEF_README_SELECTION_ROOT"
	if root := os.Getenv(helperEnv); root != "" {
		cmdScan([]string{"-json", root})
		return
	}
	for _, tt := range []struct {
		name  string
		files []string
		want  string
	}{
		{"markdown before html", []string{"README.md", "README.html"}, "README.md"},
		{"mixed case", []string{"ReadMe.Md", "README.HTML"}, "ReadMe.Md"},
		{"plain before html", []string{"README", "README.html"}, "README"},
		{"rst before html", []string{"README.rst", "README.html"}, "README.rst"},
		{"markdown extension", []string{"README.markdown", "README.html"}, "README.markdown"},
		{"text before html", []string{"README.txt", "README.html"}, "README.txt"},
		{"rmarkdown before html", []string{"README.Rmd", "README.html"}, "README.Rmd"},
		{"quarto before html", []string{"README.qmd", "README.html"}, "README.qmd"},
		{"asciidoc before html", []string{"README.adoc", "README.html"}, "README.adoc"},
		{"html fallback", []string{"README.html"}, "README.html"},
		{"custom fallback", []string{"README.custom"}, "README.custom"},
		{"docs markdown before html", []string{"docs/README.md", "docs/README.html"}, "docs/README.md"},
		{"root before docs", []string{"README.html", "docs/README.md"}, "README.html"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			for _, path := range tt.files {
				writeScanFixture(t, root, path, "# Project documentation\n")
			}
			cmd := exec.Command(os.Args[0], "-test.run=^TestScanReadmeSelection$")
			cmd.Env = append(os.Environ(), helperEnv+"="+root, "PATH=")
			out, err := cmd.Output()
			if err != nil {
				t.Fatalf("scan failed: %v", err)
			}
			var report brief.Report
			if err := json.Unmarshal(out, &report); err != nil {
				t.Fatalf("parsing scan output: %v\n%s", err, out)
			}
			if report.Resources == nil || report.Resources.Readme != tt.want {
				t.Fatalf("resources = %+v, want readme %q", report.Resources, tt.want)
			}
		})
	}
}
