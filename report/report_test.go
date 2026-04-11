package report

import (
	"bytes"
	"strings"
	"testing"

	"github.com/git-pkgs/brief"
)

func TestHumanLayout(t *testing.T) {
	r := &brief.Report{
		Version: "dev",
		Path:    "/tmp/test",
		Layout: &brief.LayoutInfo{
			SourceDirs: []string{"src", "lib"},
			TestDirs:   []string{"test"},
		},
	}

	var buf bytes.Buffer
	Human(&buf, r, false)
	out := buf.String()

	if !strings.Contains(out, "source: src/, lib/") {
		t.Errorf("layout source dirs wrong\ngot:\n%s", out)
	}
	if !strings.Contains(out, "test: test/") {
		t.Errorf("layout test dirs wrong\ngot:\n%s", out)
	}
}

func TestMarkdownLayout(t *testing.T) {
	r := &brief.Report{
		Version: "dev",
		Path:    "/tmp/test",
		Layout: &brief.LayoutInfo{
			SourceDirs: []string{"src", "lib"},
			TestDirs:   []string{"test"},
		},
	}

	var buf bytes.Buffer
	Markdown(&buf, r, false)
	out := buf.String()

	if !strings.Contains(out, "source: src/, lib/") {
		t.Errorf("markdown layout source dirs wrong\ngot:\n%s", out)
	}
	if !strings.Contains(out, "test: test/") {
		t.Errorf("markdown layout test dirs wrong\ngot:\n%s", out)
	}
}

func TestHumanGitRemotesSorted(t *testing.T) {
	r := &brief.Report{
		Version: "dev",
		Path:    "/tmp/test",
		Git: &brief.GitInfo{
			Branch: "main",
			Remotes: map[string]string{
				"upstream": "git@github.com:upstream/repo.git",
				"origin":   "git@github.com:user/repo.git",
			},
		},
	}

	var buf bytes.Buffer
	Human(&buf, r, false)
	out := buf.String()

	originIdx := strings.Index(out, "origin:")
	upstreamIdx := strings.Index(out, "upstream:")
	if originIdx < 0 || upstreamIdx < 0 {
		t.Fatalf("missing remotes in output:\n%s", out)
	}
	if originIdx > upstreamIdx {
		t.Errorf("expected origin before upstream (sorted order)")
	}
}

func TestMarkdownGitRemotesSorted(t *testing.T) {
	r := &brief.Report{
		Version: "dev",
		Path:    "/tmp/test",
		Git: &brief.GitInfo{
			Branch: "main",
			Remotes: map[string]string{
				"upstream": "git@github.com:upstream/repo.git",
				"origin":   "git@github.com:user/repo.git",
			},
		},
	}

	var buf bytes.Buffer
	Markdown(&buf, r, false)
	out := buf.String()

	originIdx := strings.Index(out, "origin:")
	upstreamIdx := strings.Index(out, "upstream:")
	if originIdx < 0 || upstreamIdx < 0 {
		t.Fatalf("missing remotes in output:\n%s", out)
	}
	if originIdx > upstreamIdx {
		t.Errorf("expected origin before upstream (sorted order)")
	}
}

func TestHumanPlatformsSorted(t *testing.T) {
	r := &brief.Report{
		Version: "dev",
		Path:    "/tmp/test",
		Platforms: &brief.PlatformInfo{
			RuntimeVersionFiles: map[string]string{
				".ruby-version": "3.3.0",
				".node-version": "20.0.0",
			},
			CIMatrixVersions: make(map[string][]string),
		},
	}

	var buf bytes.Buffer
	Human(&buf, r, false)
	out := buf.String()

	nodeIdx := strings.Index(out, ".node-version:")
	rubyIdx := strings.Index(out, ".ruby-version:")
	if nodeIdx < 0 || rubyIdx < 0 {
		t.Fatalf("missing runtime versions in output:\n%s", out)
	}
	if nodeIdx > rubyIdx {
		t.Errorf("expected .node-version before .ruby-version (sorted order)")
	}
}

func TestSanitize(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello", "hello"},
		{"hello\tworld", "hello\tworld"},
		{"hello\x1b[31mred\x1b[0m", "hello[31mred[0m"},
		{"line\nbreak", "line\nbreak"},
	}
	for _, tt := range tests {
		if got := sanitize(tt.input); got != tt.want {
			t.Errorf("sanitize(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestDepSummary(t *testing.T) {
	deps := []brief.DepInfo{
		{Name: "foo", PURL: "pkg:npm/foo", Scope: brief.ScopeRuntime, Direct: true},
		{Name: "bar", PURL: "pkg:npm/bar", Scope: brief.ScopeRuntime, Direct: false},
		{Name: "dev-dep", PURL: "pkg:npm/dev-dep", Scope: brief.ScopeDevelopment, Direct: true},
	}

	s := depSummary(deps)
	if !strings.Contains(s, "1 runtime") {
		t.Errorf("expected '1 runtime' in %q", s)
	}
	if !strings.Contains(s, "2 total") {
		t.Errorf("expected '2 total' in %q", s)
	}
	if !strings.Contains(s, "1 dev") {
		t.Errorf("expected '1 dev' in %q", s)
	}
}

func TestDepSummary_Empty(t *testing.T) {
	if s := depSummary(nil); s != "" {
		t.Errorf("expected empty string, got %q", s)
	}
}

func TestSortedKeys(t *testing.T) {
	m := map[string]int{"c": 3, "a": 1, "b": 2}
	keys := sortedKeys(m)
	if len(keys) != 3 || keys[0] != "a" || keys[1] != "b" || keys[2] != "c" {
		t.Errorf("sortedKeys() = %v, want [a b c]", keys)
	}
}

func TestJoinDirs(t *testing.T) {
	tests := []struct {
		input []string
		want  string
	}{
		{[]string{"src"}, "src/"},
		{[]string{"src", "lib"}, "src/, lib/"},
		{[]string{"src", "lib", "app"}, "src/, lib/, app/"},
	}
	for _, tt := range tests {
		if got := joinDirs(tt.input); got != tt.want {
			t.Errorf("joinDirs(%v) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func sampleThreatReport() *brief.ThreatReport {
	return &brief.ThreatReport{
		Ecosystems: []string{"ruby"},
		Stack: []brief.StackEntry{
			{Name: "Rails"},
			{Name: "Ruby"},
		},
		Threats: []brief.Threat{
			{
				ID:           "xss",
				CWE:          "CWE-79",
				OWASP:        "A03:2021",
				Title:        "Cross-Site Scripting",
				IntroducedBy: []string{"Rails"},
				Note:         "Renders data into output",
			},
			{
				ID:           "sql_injection",
				CWE:          "CWE-89",
				OWASP:        "A03:2021",
				Title:        "SQL Injection",
				IntroducedBy: []string{"ActiveRecord", "Rails"},
			},
		},
	}
}

func TestThreatHuman(t *testing.T) {
	var buf bytes.Buffer
	ThreatHuman(&buf, sampleThreatReport())
	out := buf.String()

	checks := []string{
		"Detected: ruby",
		"Stack:    Rails, Ruby",
		"xss",
		"Cross-Site Scripting",
		"CWE-79 A03:2021",
		"via Rails",
		"Renders data into output",
		"sql_injection",
		"via ActiveRecord, Rails",
	}
	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\ngot:\n%s", want, out)
		}
	}
}

func TestThreatHumanEmpty(t *testing.T) {
	var buf bytes.Buffer
	ThreatHuman(&buf, &brief.ThreatReport{})
	if !strings.Contains(buf.String(), "No security data available") {
		t.Errorf("expected empty message, got:\n%s", buf.String())
	}
}

func TestThreatJSON(t *testing.T) {
	var buf bytes.Buffer
	if err := ThreatJSON(&buf, sampleThreatReport()); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, `"id": "xss"`) {
		t.Errorf("expected xss in JSON, got:\n%s", out)
	}
	if !strings.Contains(out, `"introduced_by"`) {
		t.Errorf("expected introduced_by field, got:\n%s", out)
	}
}

func sampleSinkReport() *brief.SinkReport {
	return &brief.SinkReport{
		Sinks: []brief.SinkEntry{
			{Symbol: "eval", Tool: "Ruby", Threat: "code_injection", CWE: "CWE-95"},
			{Symbol: "system", Tool: "Ruby", Threat: "command_injection", CWE: "CWE-78", Note: "Spawns shell"},
			{Symbol: "html_safe", Tool: "Rails", Threat: "xss", CWE: "CWE-79"},
		},
	}
}

func TestSinksHuman(t *testing.T) {
	var buf bytes.Buffer
	SinksHuman(&buf, sampleSinkReport())
	out := buf.String()

	checks := []string{
		"Ruby:",
		"eval",
		"code_injection",
		"CWE-95",
		"system",
		"Spawns shell",
		"Rails:",
		"html_safe",
	}
	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\ngot:\n%s", want, out)
		}
	}

	// Ruby section should appear before Rails section (input order preserved).
	rubyIdx := strings.Index(out, "Ruby:")
	railsIdx := strings.Index(out, "Rails:")
	if rubyIdx > railsIdx {
		t.Error("expected tool sections in input order")
	}
}

func TestSinksHumanEmpty(t *testing.T) {
	var buf bytes.Buffer
	SinksHuman(&buf, &brief.SinkReport{})
	if !strings.Contains(buf.String(), "No sink data available") {
		t.Errorf("expected empty message, got:\n%s", buf.String())
	}
}

func TestSinksJSON(t *testing.T) {
	var buf bytes.Buffer
	if err := SinksJSON(&buf, sampleSinkReport()); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, `"symbol": "eval"`) {
		t.Errorf("expected eval in JSON, got:\n%s", out)
	}
}
