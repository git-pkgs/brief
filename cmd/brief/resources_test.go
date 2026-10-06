package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"testing"

	"github.com/git-pkgs/brief"
)

func TestScanResourceVariants(t *testing.T) {
	const helperEnv = "BRIEF_RESOURCE_VARIANTS_ROOT"
	if root := os.Getenv(helperEnv); root != "" {
		cmdScan([]string{"-json", root})
		return
	}
	for _, tt := range []struct {
		name  string
		files []string
		group string
		field string
		want  string
	}{
		{"conduct text", []string{"CODE-OF-CONDUCT.txt"}, "community", "code_of_conduct", "CODE-OF-CONDUCT.txt"},
		{"conduct rst", []string{"docs/Code-of-Conduct.rst"}, "community", "code_of_conduct", "docs/Code-of-Conduct.rst"},
		{"conduct adoc", []string{".github/CODE-OF-CONDUCT.adoc"}, "community", "code_of_conduct", ".github/CODE-OF-CONDUCT.adoc"},
		{"conduct underscore adoc", []string{".gitlab/CODE_OF_CONDUCT.adoc"}, "community", "code_of_conduct", ".gitlab/CODE_OF_CONDUCT.adoc"},
		{"cla no extension", []string{"CONTRIBUTOR LICENSE AGREEMENT"}, "legal", "cla", "CONTRIBUTOR LICENSE AGREEMENT"},
		{"cla markdown", []string{"docs/Contributor License Agreement.md"}, "legal", "cla", "docs/Contributor License Agreement.md"},
		{"cla text", []string{".github/CONTRIBUTOR LICENSE AGREEMENT.txt"}, "legal", "cla", ".github/CONTRIBUTOR LICENSE AGREEMENT.txt"},
		{"change no extension", []string{"CHANGE"}, "", "changelog", "CHANGE"},
		{"change markdown", []string{"CHANGE.md"}, "", "changelog", "CHANGE.md"},
		{"change text", []string{"Change.txt"}, "", "changelog", "Change.txt"},
		{"change rst", []string{"CHANGE.rst"}, "", "changelog", "CHANGE.rst"},
		{"change adoc", []string{"CHANGE.adoc"}, "", "changelog", "CHANGE.adoc"},
		{"changelog precedence", []string{"CHANGELOG.md", "CHANGE.md"}, "", "changelog", "CHANGELOG.md"},
		{"conduct similar names", []string{"CODE-OF-CONDUCT.txt.bak", "docs/CODE-OF-CONDUCT-POLICY.md"}, "community", "code_of_conduct", ""},
		{"cla similar names", []string{"CONTRIBUTOR LICENSE AGREEMENT.md.bak", "docs/CONTRIBUTOR LICENSE AGREEMENT POLICY.md"}, "legal", "cla", ""},
		{"change similar names", []string{"CHANGE.md.bak", "CHANGE-REQUEST.md"}, "", "changelog", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			writeScanFixture(t, root, "README.md", "# Example project\n")
			for _, path := range tt.files {
				writeScanFixture(t, root, path, "# Project documentation\n\nContact the maintainers through the issue tracker.\n")
			}
			cmd := exec.Command(os.Args[0], "-test.run=^TestScanResourceVariants$")
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
			got := report.Resources.Changelog
			if tt.group != "" {
				got = report.Resources.Group(tt.group)[tt.field]
			}
			if got != tt.want {
				t.Errorf("resource %s = %q, want %q", tt.field, got, tt.want)
			}
		})
	}
}
