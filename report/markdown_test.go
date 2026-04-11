package report

import (
	"bytes"
	"strings"
	"testing"

	"github.com/git-pkgs/brief"
)

func TestMarkdownBasic(t *testing.T) {
	r := &brief.Report{
		Version: "dev",
		Path:    "/tmp/test",
		Languages: []brief.Detection{
			{Name: "Go", Category: "language", Confidence: "high"},
		},
		PackageManagers: []brief.Detection{
			{
				Name:     "Go Modules",
				Category: "package_manager",
				Command:  &brief.Command{Run: "go mod download"},
				Lockfile: "go.sum",
			},
		},
		Tools: map[string][]brief.Detection{
			"test": {
				{Name: "go test", Command: &brief.Command{Run: "go test ./..."}},
			},
			"lint": {
				{Name: "golangci-lint", Command: &brief.Command{Run: "golangci-lint run"}, ConfigFiles: []string{".golangci.yml"}},
			},
		},
		Stats: brief.Stats{
			DurationMS:   1.5,
			FilesChecked: 10,
			ToolsMatched: 2,
			ToolsChecked: 50,
		},
	}

	var buf bytes.Buffer
	Markdown(&buf, r, false)
	out := buf.String()

	checks := []string{
		"# brief dev",
		"**Language:** Go",
		"**Package Manager:** Go Modules (`go mod download`)",
		"Lockfile: go.sum",
		"## Tools",
		"| Category | Tool | Command | Config |",
		"| Test | go test | `go test ./...` |",
		"| Lint | golangci-lint | `golangci-lint run` | .golangci.yml |",
		"1.5ms | 10 files checked | 2/50 tools matched",
	}
	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\ngot:\n%s", want, out)
		}
	}
}

func TestMarkdownVerbose(t *testing.T) {
	r := &brief.Report{
		Version: "dev",
		Path:    "/tmp/test",
		Tools: map[string][]brief.Detection{
			"lint": {
				{
					Name:     "golangci-lint",
					Homepage: "https://golangci-lint.run",
					Docs:     "https://golangci-lint.run/usage/",
				},
			},
		},
	}

	var buf bytes.Buffer
	Markdown(&buf, r, true)
	out := buf.String()

	if !strings.Contains(out, "[homepage](https://golangci-lint.run)") {
		t.Errorf("verbose output missing homepage link\ngot:\n%s", out)
	}
	if !strings.Contains(out, "[docs](https://golangci-lint.run/usage/)") {
		t.Errorf("verbose output missing docs link\ngot:\n%s", out)
	}
}

func TestMarkdownDiff(t *testing.T) {
	r := &brief.Report{
		Version:      "dev",
		Path:         "/tmp/test",
		DiffRef:      "main..HEAD",
		ChangedFiles: []string{"foo.go", "bar.go"},
		DiffCommits:  []string{"abc1234 fix stuff", "def5678 add thing"},
	}

	var buf bytes.Buffer
	Markdown(&buf, r, false)
	out := buf.String()

	if !strings.Contains(out, "diff main..HEAD (2 files changed)") {
		t.Errorf("missing diff ref line\ngot:\n%s", out)
	}
	if !strings.Contains(out, "- abc1234 fix stuff") {
		t.Errorf("missing commit list\ngot:\n%s", out)
	}
	if !strings.Contains(out, "- foo.go") {
		t.Errorf("missing changed files\ngot:\n%s", out)
	}
}

func TestMarkdownStyle(t *testing.T) {
	tn := true
	r := &brief.Report{
		Version: "dev",
		Path:    "/tmp/test",
		Style: &brief.StyleInfo{
			Indentation:     "tabs",
			IndentSource:    ".editorconfig",
			LineEnding:      "LF",
			TrailingNewline: &tn,
		},
	}

	var buf bytes.Buffer
	Markdown(&buf, r, false)
	out := buf.String()

	if !strings.Contains(out, "**Style:** tabs (.editorconfig) | LF | trailing newline") {
		t.Errorf("style output wrong\ngot:\n%s", out)
	}
}

func TestMarkdownResources(t *testing.T) {
	r := &brief.Report{
		Version: "dev",
		Path:    "/tmp/test",
		Resources: &brief.ResourceInfo{
			Readme:  "README.md",
			License: "LICENSE",
		},
	}

	var buf bytes.Buffer
	Markdown(&buf, r, false)
	out := buf.String()

	if !strings.Contains(out, "**Resources:**") {
		t.Errorf("missing resources header\ngot:\n%s", out)
	}
	if !strings.Contains(out, "- README.md") {
		t.Errorf("missing readme\ngot:\n%s", out)
	}
	if !strings.Contains(out, "- LICENSE") {
		t.Errorf("missing license\ngot:\n%s", out)
	}
}

func TestMarkdownGit(t *testing.T) {
	r := &brief.Report{
		Version: "dev",
		Path:    "/tmp/test",
		Git: &brief.GitInfo{
			Branch:      "main",
			CommitCount: 42,
			Remotes:     map[string]string{"origin": "git@github.com:user/repo.git"},
		},
	}

	var buf bytes.Buffer
	Markdown(&buf, r, false)
	out := buf.String()

	if !strings.Contains(out, "**Git:** branch `main`") {
		t.Errorf("missing git branch\ngot:\n%s", out)
	}
	if !strings.Contains(out, "42 commits") {
		t.Errorf("missing commit count\ngot:\n%s", out)
	}
	if !strings.Contains(out, "- origin: git@github.com:user/repo.git") {
		t.Errorf("missing remote\ngot:\n%s", out)
	}
}

func TestMarkdownLines(t *testing.T) {
	r := &brief.Report{
		Version: "dev",
		Path:    "/tmp/test",
		Lines: &brief.LineCount{
			TotalLines: 1000,
			TotalFiles: 50,
			Source:     "tokei",
		},
	}

	var buf bytes.Buffer
	Markdown(&buf, r, false)
	out := buf.String()

	if !strings.Contains(out, "**Lines:** 1000 code, 50 files (tokei)") {
		t.Errorf("missing lines\ngot:\n%s", out)
	}
}

func TestMarkdownScripts(t *testing.T) {
	r := &brief.Report{
		Version: "dev",
		Path:    "/tmp/test",
		Scripts: []brief.Script{
			{Name: "test", Run: "jest", Source: "package.json"},
			{Name: "build", Run: "tsc", Source: "package.json"},
		},
	}

	var buf bytes.Buffer
	Markdown(&buf, r, false)
	out := buf.String()

	if !strings.Contains(out, "## Scripts (package.json)") {
		t.Errorf("missing scripts header\ngot:\n%s", out)
	}
	if !strings.Contains(out, "| test | `jest` |") {
		t.Errorf("missing script row\ngot:\n%s", out)
	}
}

func TestMissingMarkdown(t *testing.T) {
	mr := &brief.MissingReport{
		Ecosystems: []string{"Go"},
		Missing: []brief.MissingCategory{
			{
				Label:        "Format",
				Suggested:    "gofmt",
				SuggestedCmd: "gofmt -w .",
				Docs:         "https://go.dev/blog/gofmt",
			},
		},
	}

	var buf bytes.Buffer
	MissingMarkdown(&buf, mr)
	out := buf.String()

	checks := []string{
		"**Detected:** Go",
		"## Missing recommended tooling",
		"| Category | Suggested | Command | Docs |",
		"| Format | gofmt | `gofmt -w .` |",
	}
	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\ngot:\n%s", want, out)
		}
	}
}

func TestMissingMarkdownEmpty(t *testing.T) {
	mr := &brief.MissingReport{}

	var buf bytes.Buffer
	MissingMarkdown(&buf, mr)
	out := buf.String()

	if !strings.Contains(out, "No missing recommended tooling detected.") {
		t.Errorf("expected empty message\ngot:\n%s", out)
	}
}

func TestThreatMarkdown(t *testing.T) {
	tr := &brief.ThreatReport{
		Ecosystems: []string{"ruby"},
		Threats: []brief.Threat{
			{ID: "xss", CWE: "CWE-79", OWASP: "A03:2021", Title: "Cross-Site Scripting", IntroducedBy: []string{"Rails"}},
		},
	}

	var buf bytes.Buffer
	ThreatMarkdown(&buf, tr)
	out := buf.String()

	checks := []string{
		"**Detected:** ruby",
		"| Threat | CWE | OWASP | Introduced by |",
		"| Cross-Site Scripting | CWE-79 | A03:2021 | Rails |",
	}
	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\ngot:\n%s", want, out)
		}
	}
}

func TestThreatMarkdownEmpty(t *testing.T) {
	var buf bytes.Buffer
	ThreatMarkdown(&buf, &brief.ThreatReport{})
	if !strings.Contains(buf.String(), "No threat categories match") {
		t.Errorf("expected empty message\ngot:\n%s", buf.String())
	}
}

func TestSinksMarkdown(t *testing.T) {
	sr := &brief.SinkReport{
		Sinks: []brief.SinkEntry{
			{Symbol: "eval", Tool: "Ruby", Threat: "code_injection", CWE: "CWE-95"},
		},
	}

	var buf bytes.Buffer
	SinksMarkdown(&buf, sr)
	out := buf.String()

	checks := []string{
		"| Tool | Symbol | Threat | CWE |",
		"| Ruby | `eval` | code_injection | CWE-95 |",
	}
	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\ngot:\n%s", want, out)
		}
	}
}

func TestSinksMarkdownEmpty(t *testing.T) {
	var buf bytes.Buffer
	SinksMarkdown(&buf, &brief.SinkReport{})
	if !strings.Contains(buf.String(), "No sink data available") {
		t.Errorf("expected empty message\ngot:\n%s", buf.String())
	}
}
