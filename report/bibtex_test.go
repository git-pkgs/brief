package report

import (
	"bytes"
	"strings"
	"testing"

	"github.com/git-pkgs/brief"
)

func TestBibTeXOutputEscapesAndBounds(t *testing.T) {
	info := &brief.BibTeXInfo{
		Path: "CITATION.bib", ParseStatus: "parsed",
		Diagnostics: []brief.CitationDiagnostic{{Code: "missing_comma", Message: "expected comma\nafter key", Line: 2, Column: 3}},
	}
	for range 100 {
		info.Entries = append(info.Entries, brief.BibTeXEntry{
			Type: "misc", Key: "k_1", Title: "<script>*title* [x](javascript:alert(1))\x1b\nInjected:\u202esecret" + strings.Repeat("x", 1000),
			Authors: []brief.CitationAuthor{{Name: "Team"}, {Name: "Team"}, {Name: "Team"}, {Name: "Team"}},
		})
	}
	r := &brief.Report{Resources: &brief.ResourceInfo{BibTeX: info}}
	for _, markdown := range []bool{false, true} {
		for _, verbose := range []bool{false, true} {
			var out bytes.Buffer
			if markdown {
				Markdown(&out, r, verbose)
			} else {
				Human(&out, r, verbose)
			}
			text := out.String()
			for _, unwanted := range []string{"\x1b", "\nInjected:", "\u202e", strings.Repeat("x", 301), "\nafter key"} {
				if strings.Contains(text, unwanted) {
					t.Fatalf("markdown=%v verbose=%v: output contains %q", markdown, verbose, unwanted)
				}
			}
			if markdown && strings.Contains(text, "<script>") {
				t.Fatalf("unescaped markdown: %s", text)
			}
			for _, want := range []string{"CITATION.bib, 100 entries", "and 1 more", "CITATION.bib:2:3 missing"} {
				if !strings.Contains(text, want) {
					t.Fatalf("markdown=%v verbose=%v: missing %q", markdown, verbose, want)
				}
			}
		}
	}
}
