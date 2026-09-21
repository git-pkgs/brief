package report

import (
	"bytes"
	"strings"
	"testing"

	"github.com/git-pkgs/brief"
)

func TestCitationOutputEscapesAndBounds(t *testing.T) {
	r := &brief.Report{Resources: &brief.ResourceInfo{Citation: &brief.CitationInfo{
		Path: "CITATION.cff", ParseStatus: "parsed", ValidationStatus: "invalid",
		Title:       "<script>*title* [click](javascript:alert(1))\x1b\nInjected:\u2028line\u202esecret",
		Abstract:    strings.Repeat("x", 10000),
		Diagnostics: []brief.CitationDiagnostic{{Code: "type", Path: "title", Message: "wrong\nshape", Line: 2, Column: 3}},
	}}}
	for range 100 {
		r.Resources.Citation.Authors = append(r.Resources.Citation.Authors, brief.CitationAuthor{Name: "Team"})
	}
	for _, markdown := range []bool{false, true} {
		var out bytes.Buffer
		if markdown {
			Markdown(&out, r, true)
		} else {
			Human(&out, r, true)
		}
		text := out.String()
		for _, unwanted := range []string{"\x1b", "\nInjected:", "\u2028", "\u202e", strings.Repeat("x", 301)} {
			if strings.Contains(text, unwanted) {
				t.Errorf("unsafe or unbounded output contains %q", unwanted)
			}
		}
		if !strings.Contains(text, "and 97 more") || !strings.Contains(text, "CITATION.cff:2:3") {
			t.Fatalf("missing author count or diagnostic location: %s", text)
		}
		if markdown && (strings.Contains(text, "<script>") || strings.Contains(text, "[click](")) {
			t.Fatalf("active Markdown or HTML: %s", text)
		}
	}
}
