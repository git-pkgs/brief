package report

import (
	"bytes"
	"strings"
	"testing"

	"github.com/git-pkgs/brief"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTML(t *testing.T) {
	r := &brief.Report{
		Version: "test",
		Path:    "/tmp/proj",
		Languages: []brief.Detection{
			{Name: "Go", Homepage: "https://go.dev", Description: "Compiled language"},
		},
		PackageManagers: []brief.Detection{
			{Name: "Go Modules", Command: &brief.Command{Run: "go mod download"}, Lockfile: "go.sum"},
		},
		Tools: map[string][]brief.Detection{
			"test": {{Name: "go test", Command: &brief.Command{Run: "go test ./..."}, Docs: "https://pkg.go.dev/testing"}},
			"lint": {{Name: "golangci-lint", ConfigFiles: []string{".golangci.yml"}}},
		},
		Scripts: []brief.Script{{Name: "build", Run: "go build ./...", Source: "Makefile"}},
		Style:   &brief.StyleInfo{Indentation: "tabs", LineEnding: "LF"},
		Layout:  &brief.LayoutInfo{SourceDirs: []string{"cmd"}},
		Platforms: &brief.PlatformInfo{
			CIMatrixOS: []string{"ubuntu-latest"},
		},
		Resources: &brief.ResourceInfo{
			Readme:      "README.md",
			License:     "LICENSE",
			LicenseType: "MIT",
			Community:   map[string]string{"contributing": "CONTRIBUTING.md"},
		},
		Git: &brief.GitInfo{
			Branch:        "main",
			DefaultBranch: "main",
			Remotes:       map[string]string{"origin": "git@github.com:acme/proj.git"},
			CommitCount:   1234,
		},
		Lines: &brief.LineCount{
			TotalFiles: 42,
			TotalLines: 12345,
			ByLanguage: map[string]int{"Go": 10000, "Markdown": 2345},
			Source:     "scc",
		},
		Dependencies: []brief.DepInfo{
			{Name: "github.com/a/b", Version: "v1.0.0", Scope: brief.ScopeRuntime, Direct: true},
			{Name: "github.com/c/d", Version: "v2.0.0", Scope: brief.ScopeDevelopment, Direct: true},
			{Name: "github.com/e/f", Version: "v0.1.0", Scope: brief.ScopeRuntime, Direct: false},
		},
		Stats: brief.Stats{DurationMS: 12.3, FilesChecked: 42, ToolsMatched: 4, ToolsChecked: 100},
	}

	var buf bytes.Buffer
	require.NoError(t, HTML(&buf, r))
	out := buf.String()

	assert.True(t, strings.HasPrefix(out, "<!doctype html>"))
	for _, s := range []string{
		"<title>proj · brief</title>",
		"https://github.com/acme/proj",
		"basecoat-css",
		"lucide",
		"data-lucide=",
		"12,345",
		"1,234",
		"go test ./...",
		"data-copy=",
		".golangci.yml",
		"github.com/a/b",
		"github.com/e/f",
		`role="tablist"`,
		"README.md",
		"CONTRIBUTING.md",
		"MIT",
		"tabs",
		"ubuntu-latest",
	} {
		assert.Contains(t, out, s, "missing %q", s)
	}
}

func TestHTMLEmptyReport(t *testing.T) {
	r := &brief.Report{Version: "test", Path: ".", Tools: map[string][]brief.Detection{}}
	var buf bytes.Buffer
	require.NoError(t, HTML(&buf, r))
	assert.Contains(t, buf.String(), "<title>project · brief</title>")
}

func TestProjectTitle(t *testing.T) {
	cases := []struct {
		name string
		r    *brief.Report
		want string
	}{
		{"ssh remote", &brief.Report{Git: &brief.GitInfo{Remotes: map[string]string{"origin": "git@github.com:acme/widget.git"}}}, "widget"},
		{"https remote", &brief.Report{Git: &brief.GitInfo{Remotes: map[string]string{"origin": "https://gitlab.com/group/sub/thing"}}}, "thing"},
		{"path fallback", &brief.Report{Path: "/home/u/code/myproj"}, "myproj"},
		{"empty", &brief.Report{Path: "."}, "project"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, projectTitle(tc.r))
		})
	}
}

func TestRepoWebURL(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"git@github.com:acme/proj.git", "https://github.com/acme/proj"},
		{"https://github.com/acme/proj.git", "https://github.com/acme/proj"},
		{"http://example.com/r", "http://example.com/r"},
		{"ssh://weird", ""},
	}
	for _, tc := range cases {
		got := repoWebURL(&brief.Report{Git: &brief.GitInfo{Remotes: map[string]string{"origin": tc.in}}})
		assert.Equal(t, tc.want, got, "input %q", tc.in)
	}
	assert.Equal(t, "", repoWebURL(&brief.Report{}))
}

func TestLanguageSlices(t *testing.T) {
	lc := &brief.LineCount{ByLanguage: map[string]int{
		"Go": 800, "Ruby": 150, "License": 50, "Plain Text": 10,
	}}
	got := languageSlices(lc)
	require.Len(t, got, 2)
	assert.Equal(t, "Go", got[0].Name)
	assert.InDelta(t, 84.2, got[0].Percent, 0.1)
	assert.Equal(t, "Ruby", got[1].Name)

	assert.Nil(t, languageSlices(nil))
	assert.Nil(t, languageSlices(&brief.LineCount{ByLanguage: map[string]int{"License": 10}}))
}

func TestMajorLanguages(t *testing.T) {
	langs := []brief.Detection{{Name: "TypeScript"}, {Name: "JavaScript"}, {Name: "Elm"}}
	lc := &brief.LineCount{ByLanguage: map[string]int{"TypeScript": 900, "JavaScript": 50, "Markdown": 50}}

	got := majorLanguages(langs, lc)
	require.Len(t, got, 2)
	assert.Equal(t, "TypeScript", got[0].Name)
	assert.Equal(t, "Elm", got[1].Name) // no line entry, kept

	assert.Equal(t, langs, majorLanguages(langs, nil))
	assert.Equal(t, langs, majorLanguages(langs, &brief.LineCount{}))

	tiny := []brief.Detection{{Name: "JavaScript"}}
	assert.Equal(t, tiny, majorLanguages(tiny, lc)) // nothing passes, fall back
}

func TestLanguageSlicesOther(t *testing.T) {
	by := map[string]int{}
	for i, n := range []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"} {
		by[n] = 100 - i
	}
	got := languageSlices(&brief.LineCount{ByLanguage: by})
	require.Len(t, got, 9)
	assert.Equal(t, "Other", got[8].Name)
}

func TestSplitDeps(t *testing.T) {
	d, tr := splitDeps([]brief.DepInfo{
		{Name: "a", Direct: true},
		{Name: "b", Direct: false},
		{Name: "c", Direct: true},
	})
	assert.Len(t, d, 2)
	assert.Len(t, tr, 1)
}

func TestComma(t *testing.T) {
	f := htmlFuncs["comma"].(func(int) string)
	assert.Equal(t, "0", f(0))
	assert.Equal(t, "12", f(12))
	assert.Equal(t, "999", f(999))
	assert.Equal(t, "1,000", f(1000))
	assert.Equal(t, "12,345", f(12345))
	assert.Equal(t, "1,234,567", f(1234567))
}
