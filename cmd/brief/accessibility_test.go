package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"testing"

	"github.com/git-pkgs/brief"
)

func TestScanAccessibility(t *testing.T) {
	const helperEnv = "BRIEF_ACCESSIBILITY_HELPER_ROOT"
	if root := os.Getenv(helperEnv); root != "" {
		cmdScan([]string{"-json", root})
		return
	}
	for _, tt := range []struct {
		name  string
		files []string
		want  string
	}{
		{"root", []string{"ACCESSIBILITY.md"}, "ACCESSIBILITY.md"},
		{"github", []string{".github/ACCESSIBILITY.md"}, ".github/ACCESSIBILITY.md"},
		{"docs", []string{"docs/ACCESSIBILITY.md"}, "docs/ACCESSIBILITY.md"},
		{"gitlab", []string{".gitlab/ACCESSIBILITY.md"}, ".gitlab/ACCESSIBILITY.md"},
		{"no extension", []string{"ACCESSIBILITY"}, "ACCESSIBILITY"},
		{"text", []string{".github/ACCESSIBILITY.txt"}, ".github/ACCESSIBILITY.txt"},
		{"restructuredtext", []string{"docs/ACCESSIBILITY.rst"}, "docs/ACCESSIBILITY.rst"},
		{"asciidoc", []string{".gitlab/ACCESSIBILITY.adoc"}, ".gitlab/ACCESSIBILITY.adoc"},
		{"mixed case", []string{"docs/Accessibility.MD"}, "docs/Accessibility.MD"},
		{"root precedence", []string{"ACCESSIBILITY.md", ".github/ACCESSIBILITY.md", "docs/ACCESSIBILITY.md"}, "ACCESSIBILITY.md"},
		{"github precedence", []string{".github/ACCESSIBILITY.md", "docs/ACCESSIBILITY.md"}, ".github/ACCESSIBILITY.md"},
		{"docs precedence", []string{"docs/ACCESSIBILITY.md", ".gitlab/ACCESSIBILITY.md"}, "docs/ACCESSIBILITY.md"},
		{"similar names", []string{"ACCESSIBILITY.md.bak", "docs/Accessibility-Testing.md"}, ""},
		{"unrelated directory", []string{"src/ACCESSIBILITY.md"}, ""},
		{"absent", nil, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			writeScanFixture(t, root, "README.md", "# Example project\n")
			for _, path := range tt.files {
				writeScanFixture(t, root, path, "# Accessibility\n\nReport accessibility barriers through the issue tracker.\n")
			}
			cmd := exec.Command(os.Args[0], "-test.run=^TestScanAccessibility$")
			cmd.Env = append(os.Environ(), helperEnv+"="+root, "PATH=")
			out, err := cmd.Output()
			if err != nil {
				t.Fatalf("scan failed: %v", err)
			}
			var report brief.Report
			if err := json.Unmarshal(out, &report); err != nil {
				t.Fatalf("parsing scan output: %v\n%s", err, out)
			}
			if report.Resources == nil {
				t.Fatal("expected resources")
			}
			if got := report.Resources.Community["accessibility"]; got != tt.want {
				t.Errorf("community.accessibility = %q, want %q", got, tt.want)
			}
		})
	}
}
