package detect

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/git-pkgs/brief"
)

func TestCodemetaMetadata(t *testing.T) {
	t.Setenv("PATH", "")
	r, err := New(loadKB(t), "../testdata/codemeta-project").Run()
	if err != nil {
		t.Fatal(err)
	}
	info := r.Resources.Codemeta
	if info == nil || info.ParseStatus != "parsed" || info.ValidationStatus != "valid" {
		t.Fatalf("codemeta: %+v", info)
	}
	if info.Path != "codemeta.json" || r.Resources.Metadata["codemeta"] != info.Path || info.ContextVersion != "3.0" || info.Name != "Example Software" || info.Description != "Software described with CodeMeta." || info.Version != "1.2.0" {
		t.Fatalf("metadata: %+v", info)
	}
	if !reflect.DeepEqual(info.CodeRepository, []string{"https://github.com/example/codemeta-project"}) || !reflect.DeepEqual(info.Licenses, []string{"https://spdx.org/licenses/MIT.html"}) || !reflect.DeepEqual(info.Keywords, []string{"metadata", "science"}) || !reflect.DeepEqual(info.ProgrammingLanguages, []string{"Go"}) {
		t.Fatalf("lists: %+v", info)
	}
	if len(info.Authors) != 2 || info.Authors[0].Name != "Ada Lovelace" || info.Authors[0].Kind != "person" || info.Authors[1].Name != "Research Team" || info.Authors[1].Role != "creator" || info.Authors[1].Kind != "role" {
		t.Fatalf("authors: %+v", info.Authors)
	}
}

func TestCodemetaOutcomes(t *testing.T) {
	for _, tc := range []struct{ name, content, parse, validation, code string }{
		{"malformed", `{"name":`, "syntax_error", "", "syntax"},
		{"wrong root", `[]`, "type_error", "", "root_type"},
		{"unsupported context", `{"@context":"https://w3id.org/codemeta/4.0","name":"Example"}`, "parsed", "unsupported_version", "unsupported_version"},
		{"invalid metadata", `{"@context":"https://w3id.org/codemeta/3.0","name":4}`, "parsed", "invalid", "value_type"},
		{"unsupported context with field error", `{"@context":"https://w3id.org/codemeta/4.0","@id":4}`, "parsed", "unsupported_version", "unsupported_version"},
		{"oversized", `{"name":"` + strings.Repeat("x", codemetaByteLimit) + `"}`, "limit_exceeded", "", "byte_limit"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := scanCodemeta(t, tc.content)
			info := r.Resources.Codemeta
			if info == nil || info.ParseStatus != tc.parse || info.ValidationStatus != tc.validation {
				t.Fatalf("outcome: %+v", info)
			}
			if len(info.Diagnostics) == 0 || info.Diagnostics[0].Code != tc.code {
				t.Fatalf("diagnostics: %+v", info.Diagnostics)
			}
			if r.Resources.Metadata["codemeta"] != "codemeta.json" {
				t.Fatal("source path lost")
			}
		})
	}
}

func TestCodemetaDiffFilter(t *testing.T) {
	t.Setenv("PATH", "")
	r, err := New(loadKB(t), "../testdata/codemeta-project").Run()
	if err != nil {
		t.Fatal(err)
	}
	kb := loadKB(t)
	changed := FilterByChangedFiles(r, kb, []string{"codemeta.json"})
	if changed.Resources == nil || changed.Resources.Codemeta == nil || changed.Resources.Codemeta.Name != "Example Software" {
		t.Fatal("changed codemeta omitted")
	}
	unrelated := FilterByChangedFiles(r, kb, []string{"main.go"})
	if unrelated.Resources != nil && unrelated.Resources.Codemeta != nil {
		t.Fatal("unrelated change retained codemeta")
	}
}

func scanCodemeta(t *testing.T, content string) *brief.Report {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "codemeta.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	r, err := New(loadKB(t), dir).Run()
	if err != nil {
		t.Fatal(err)
	}
	if r.Resources == nil {
		t.Fatal("missing resources")
	}
	return r
}
