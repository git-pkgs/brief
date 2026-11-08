package kb_test

import (
	"testing"

	"github.com/git-pkgs/brief/kb"
)

func TestMatchPathPattern(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		pattern string
		path    string
		want    bool
	}{
		{pattern: "**/qodana.y*ml", path: "qodana.yml", want: true},
		{pattern: "**/qodana.y*ml", path: "config/qodana.yaml", want: true},
		{pattern: "*.spdx.json", path: "nested/project.spdx.json", want: false},
	} {
		if got := kb.MatchPathPattern(test.pattern, test.path); got != test.want {
			t.Errorf("MatchPathPattern(%q, %q) = %t, want %t", test.pattern, test.path, got, test.want)
		}
	}
}

func TestToolMatchesPath(t *testing.T) {
	t.Parallel()

	tool := &kb.ToolDef{
		Detect: kb.DetectInfo{Files: []string{"**/sonar-project.properties"}},
		Config: kb.ConfigInfo{Files: []string{".sonarcloud.properties"}},
	}
	if !tool.MatchesPath("services/api/sonar-project.properties") {
		t.Fatal("nested Sonar config did not match")
	}
	if tool.MatchesPath("services/api/qodana.yaml") {
		t.Fatal("unrelated config matched")
	}
}

func TestResourceMatchesPath(t *testing.T) {
	t.Parallel()

	policy := kb.ResourceInfo{
		Patterns: []string{"security.md"},
		Dirs:     []string{".github", "docs"},
	}
	if !policy.MatchesPath(".GitHub/SECURITY.MD") {
		t.Fatal("configured resource directory did not match case-insensitively")
	}
	if policy.MatchesPath("packages/api/SECURITY.md") {
		t.Fatal("unconfigured resource directory matched")
	}

	sbom := kb.ResourceInfo{Patterns: []string{"*.spdx.json"}, Recursive: true}
	if !sbom.MatchesPath("packages/api/service.SPDX.JSON") {
		t.Fatal("recursive resource did not match nested path")
	}
}

func TestSecurityDocumentResourcesMatchExpectedPaths(t *testing.T) {
	t.Parallel()

	base := loadKB(t)
	tests := []struct {
		field   string
		matches []string
		misses  []string
	}{
		{
			field: "threat_model",
			matches: []string{
				"THREAT_MODEL.md",
				"docs/design/threat-model.rst",
				"agents/references/repository-threat-model.md",
			},
			misses: []string{"docs/threat-modeling.md", "src/repository_threat_model.go"},
		},
		{
			field:   "audit",
			matches: []string{"AUDIT.md"},
			misses:  []string{"docs/cli/audit.md"},
		},
		{
			field:   "security_audit",
			matches: []string{"SECURITY-AUDIT.md", "docs/audits/security-review-2025.pdf"},
			misses:  []string{"docs/cli/audit.md", "src/security-reviewer.go"},
		},
		{
			field:   "disclosure",
			matches: []string{"DISCLOSURE.md", ".github/disclosure.adoc"},
			misses:  []string{"packages/api/disclosure.md"},
		},
	}
	for _, test := range tests {
		resource := base.Resource("security", test.field)
		if resource == nil {
			t.Errorf("security resource %q not found", test.field)
			continue
		}
		for _, filePath := range test.matches {
			if !resource.MatchesPath(filePath) {
				t.Errorf("security resource %q did not match %q", test.field, filePath)
			}
		}
		for _, filePath := range test.misses {
			if resource.MatchesPath(filePath) {
				t.Errorf("security resource %q matched %q", test.field, filePath)
			}
		}
	}
}
