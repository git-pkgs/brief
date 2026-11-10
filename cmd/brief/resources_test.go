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
		{"changelog long markdown", []string{"CHANGELOG.markdown"}, "", "changelog", "CHANGELOG.markdown"},
		{"news long markdown", []string{"News.markdown"}, "", "changelog", "News.markdown"},
		{"changelog versioned", []string{"CHANGELOG-1.x.md"}, "", "changelog", "CHANGELOG-1.x.md"},
		{"changelog versioned rst", []string{"ChangeLog-2024.rst"}, "", "changelog", "ChangeLog-2024.rst"},
		{"changelog unversioned precedence", []string{"CHANGELOG-1.x.md", "CHANGELOG.md"}, "", "changelog", "CHANGELOG.md"},
		{"changelog source files", []string{"changelog.go", "changelog-parser.go", "history.go"}, "", "changelog", ""},
		{"conduct similar names", []string{"CODE-OF-CONDUCT.txt.bak", "docs/CODE-OF-CONDUCT-POLICY.md"}, "community", "code_of_conduct", ""},
		{"cla similar names", []string{"CONTRIBUTOR LICENSE AGREEMENT.md.bak", "docs/CONTRIBUTOR LICENSE AGREEMENT POLICY.md"}, "legal", "cla", ""},
		{"change similar names", []string{"CHANGE.md.bak", "CHANGE-REQUEST.md"}, "", "changelog", ""},
		{"accessibility root", []string{"ACCESSIBILITY.md"}, "community", "accessibility", "ACCESSIBILITY.md"},
		{"accessibility github", []string{".github/ACCESSIBILITY.md"}, "community", "accessibility", ".github/ACCESSIBILITY.md"},
		{"accessibility docs", []string{"docs/ACCESSIBILITY.md"}, "community", "accessibility", "docs/ACCESSIBILITY.md"},
		{"accessibility gitlab", []string{".gitlab/ACCESSIBILITY.md"}, "community", "accessibility", ".gitlab/ACCESSIBILITY.md"},
		{"accessibility no extension", []string{"ACCESSIBILITY"}, "community", "accessibility", "ACCESSIBILITY"},
		{"accessibility text", []string{".github/ACCESSIBILITY.txt"}, "community", "accessibility", ".github/ACCESSIBILITY.txt"},
		{"accessibility rst", []string{"docs/ACCESSIBILITY.rst"}, "community", "accessibility", "docs/ACCESSIBILITY.rst"},
		{"accessibility adoc", []string{".gitlab/ACCESSIBILITY.adoc"}, "community", "accessibility", ".gitlab/ACCESSIBILITY.adoc"},
		{"accessibility mixed case", []string{"docs/Accessibility.MD"}, "community", "accessibility", "docs/Accessibility.MD"},
		{"accessibility root precedence", []string{"ACCESSIBILITY.md", ".github/ACCESSIBILITY.md", "docs/ACCESSIBILITY.md"}, "community", "accessibility", "ACCESSIBILITY.md"},
		{"accessibility github precedence", []string{".github/ACCESSIBILITY.md", "docs/ACCESSIBILITY.md"}, "community", "accessibility", ".github/ACCESSIBILITY.md"},
		{"accessibility docs precedence", []string{"docs/ACCESSIBILITY.md", ".gitlab/ACCESSIBILITY.md"}, "community", "accessibility", "docs/ACCESSIBILITY.md"},
		{"accessibility similar names", []string{"ACCESSIBILITY.md.bak", "docs/Accessibility-Testing.md"}, "community", "accessibility", ""},
		{"accessibility unrelated directory", []string{"src/ACCESSIBILITY.md"}, "community", "accessibility", ""},
		{"accessibility absent", nil, "community", "accessibility", ""},
		{"citation inst mixed case", []string{"inst/Citation.txt"}, "metadata", "citation", "inst/Citation.txt"},
		{"citation root precedence", []string{"CITATION.txt", "inst/CITATION.cff"}, "metadata", "citation", "CITATION.txt"},
		{"citation inst similar names", []string{"inst/CITATION.bak", "inst/CITATIONS", "inst/CITATION-guide.md"}, "metadata", "citation", ""},
		{"citation unrelated directory", []string{"src/CITATION", "inst/doc/CITATION"}, "metadata", "citation", ""},
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
