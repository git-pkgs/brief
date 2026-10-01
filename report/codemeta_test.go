package report

import (
	"bytes"
	"strings"
	"testing"

	"github.com/git-pkgs/brief"
)

func TestCodemetaOutput(t *testing.T) {
	r := &brief.Report{Resources: &brief.ResourceInfo{Codemeta: &brief.CodemetaInfo{
		Path: "codemeta.json", ParseStatus: "parsed", ValidationStatus: "invalid", ContextVersion: "3.0",
		Name: "Example *Software*", Version: "1.2.0", Description: "A <script> example", Licenses: []string{"MIT"},
		Authors: []brief.CodemetaAuthor{{Name: "Ada Lovelace", Kind: "person"}},
		Diagnostics: []brief.CodemetaDiagnostic{
			{Code: "value_type", Path: "name", Message: "expected text", Line: 4, Column: 9},
			{Code: "syntax", Message: "expected a JSON value", Line: 1, Column: 9},
		},
	}}}
	var human bytes.Buffer
	Human(&human, r, true)
	for _, want := range []string{"Example *Software*", "Ada Lovelace", "3.0 invalid", "1.2.0", "A <script> example", "codemeta.json:4:9 name value_type: expected text", "codemeta.json:1:9 syntax: expected a JSON value"} {
		if !strings.Contains(human.String(), want) {
			t.Errorf("human output missing %q: %s", want, human.String())
		}
	}
	var markdown bytes.Buffer
	Markdown(&markdown, r, true)
	for _, want := range []string{"**CodeMeta:**", "Example \\*Software\\*", "Ada Lovelace", "&lt;script&gt;", "codemeta.json:4:9 name value\\_type: expected text"} {
		if !strings.Contains(markdown.String(), want) {
			t.Errorf("Markdown output missing %q: %s", want, markdown.String())
		}
	}
	if strings.Contains(markdown.String(), "<script>") {
		t.Fatal("Markdown output contains active HTML")
	}
}
