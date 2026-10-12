package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/git-pkgs/brief"
)

func TestBibTeXCLI(t *testing.T) {
	root, err := filepath.Abs("../../testdata/bibtex-project")
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"--json", "--human", "--markdown", "--human --verbose", "--markdown --verbose"} {
		t.Run(mode, func(t *testing.T) {
			out := string(citationCLI(t, root, mode))
			if mode == "--json" {
				var r brief.Report
				if err := json.Unmarshal([]byte(out), &r); err != nil {
					t.Fatal(err)
				}
				if r.Resources == nil || r.Resources.BibTeX == nil || len(r.Resources.BibTeX.Entries) != 2 || r.Resources.ReadmeBibTeX == nil || r.Resources.ReadmeBibTeX.Entries[0].Key != "dvorak_2022" {
					t.Fatalf("bibtex output: %s", out)
				}
				return
			}
			for _, want := range []string{"CITATION.bib, 2 entries", "README.md, 1 entry", "Example: A GPU toolkit", "Ana Dvořák", "Jan van der Berg", "Example Research Consortium", "10.5281/zenodo.1234567", "Scaling example systems"} {
				if !strings.Contains(out, want) {
					t.Errorf("missing %q in output: %s", want, out)
				}
			}
			verbose := strings.Contains(mode, "verbose")
			if strings.Contains(out, "Journal of Open Source Software 2023") != verbose || strings.Contains(out, "2201.01234") != verbose {
				t.Fatalf("verbose=%v entry detail: %s", verbose, out)
			}
		})
	}
}

func TestBibTeXCLISchema(t *testing.T) {
	var schema struct {
		Defs map[string]struct {
			Properties map[string]json.RawMessage `json:"properties"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(citationCLI(t, t.TempDir(), "schema"), &schema); err != nil {
		t.Fatal(err)
	}
	for name, fields := range map[string][]string{
		"resourceinfo": {"bibtex", "readme_bibtex"}, "bibtexinfo": {"parse_status", "encoding", "entries", "diagnostics"},
		"bibtexentry": {"type", "key", "title", "authors", "doi", "line"},
	} {
		for _, field := range fields {
			if _, ok := schema.Defs[name].Properties[field]; !ok {
				t.Errorf("schema missing %s.%s", name, field)
			}
		}
	}
}

func TestBibTeXCLIDiff(t *testing.T) {
	root := t.TempDir()
	const content = "@misc{k, title = {First}}\n"
	writeScanFixture(t, root, "CITATION.bib", content)
	runGitFixture(t, root, "init", "-q")
	runGitFixture(t, root, "add", "CITATION.bib")
	runGitFixture(t, root, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-q", "-m", "initial")
	writeScanFixture(t, root, "CITATION.bib", strings.Replace(content, "First", "Second", 1))
	var r brief.Report
	if err := json.Unmarshal(citationCLI(t, root, "diff"), &r); err != nil {
		t.Fatal(err)
	}
	if r.Resources == nil || r.Resources.BibTeX == nil || r.Resources.BibTeX.Entries[0].Title != "Second" {
		t.Fatalf("changed bibtex missing: %+v", r.Resources)
	}
}

func TestBibTeXCLIBesideCFF(t *testing.T) {
	root := t.TempDir()
	writeScanFixture(t, root, "CITATION.cff", "cff-version: 1.2.0\nmessage: Please cite\ntitle: Example\nauthors:\n  - family-names: Doe\n")
	writeScanFixture(t, root, "CITATION.bib", "@misc{k, title = {Paper}}\n")
	var r brief.Report
	if err := json.Unmarshal(citationCLI(t, root, "--json"), &r); err != nil {
		t.Fatal(err)
	}
	if r.Resources.Citation == nil || r.Resources.Citation.Title != "Example" {
		t.Fatalf("CFF missing: %+v", r.Resources.Citation)
	}
	if info := r.Resources.BibTeX; info == nil || len(info.Entries) != 1 || info.Entries[0].Title != "Paper" {
		t.Fatalf("BibTeX missing: %+v", info)
	}
}

func TestBibTeXCLIExtractionLimits(t *testing.T) {
	root := t.TempDir()
	const content = "@misc{k, title = {Example}}\n"
	for _, name := range []string{"CITATION.md", "README.md"} {
		writeScanFixture(t, root, name, content+strings.Repeat("\n", 1<<20))
	}
	var r brief.Report
	if err := json.Unmarshal(citationCLI(t, root, "--json"), &r); err != nil {
		t.Fatal(err)
	}
	for _, info := range []*brief.BibTeXInfo{r.Resources.BibTeX, r.Resources.ReadmeBibTeX} {
		if info == nil || info.ParseStatus != "limit_exceeded" || len(info.Diagnostics) != 1 || info.Diagnostics[0].Code != "byte_limit" {
			t.Fatalf("limit diagnostic missing: %+v", info)
		}
	}
	for _, mode := range []string{"--human", "--markdown"} {
		out := string(citationCLI(t, root, mode))
		for _, want := range []string{"CITATION.md limit", "README.md limit", "byte"} {
			if !strings.Contains(out, want) {
				t.Errorf("%s: missing %q in %s", mode, want, out)
			}
		}
	}
}
