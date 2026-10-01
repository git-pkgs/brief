package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/git-pkgs/brief"
)

func TestCodemetaCLIProjection(t *testing.T) {
	root := t.TempDir()
	const content = `{
		"@context":"https://w3id.org/codemeta/3.0",
		"name":"Example",
		"codeRepository":{"id":"https://example.org/repo"},
		"license":{"id":"https://spdx.org/licenses/MIT"},
		"programmingLanguage":{"schema:name":"Go"},
		"keywords":{"@set":["science","metadata"]},
		"author":{
			"@type":"Role","name":"Development","roleName":"developer",
			"author":[
				{"@type":"Person","givenName":"Ada","familyName":"Lovelace"},
				{"@type":"Person","givenName":"Grace","familyName":"Hopper"}
			]
		}
	}`
	if err := os.WriteFile(filepath.Join(root, "codemeta.json"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	var r brief.Report
	if err := json.Unmarshal(citationCLI(t, root, "--json"), &r); err != nil {
		t.Fatal(err)
	}
	if r.Resources == nil || r.Resources.Codemeta == nil {
		t.Fatal("missing CodeMeta metadata")
	}
	info := r.Resources.Codemeta
	if info.ValidationStatus != "valid" {
		t.Fatalf("invalid metadata: %+v", info)
	}
	for field, pair := range map[string][2][]string{
		"repository": {info.CodeRepository, {"https://example.org/repo"}},
		"licenses":   {info.Licenses, {"https://spdx.org/licenses/MIT"}},
		"languages":  {info.ProgrammingLanguages, {"Go"}},
		"keywords":   {info.Keywords, {"science", "metadata"}},
	} {
		if !reflect.DeepEqual(pair[0], pair[1]) {
			t.Errorf("%s = %v, want %v", field, pair[0], pair[1])
		}
	}
	wantAuthors := []brief.CodemetaAuthor{
		{Name: "Ada Lovelace", GivenName: "Ada", FamilyName: "Lovelace", Role: "developer", Kind: "person"},
		{Name: "Grace Hopper", GivenName: "Grace", FamilyName: "Hopper", Role: "developer", Kind: "person"},
	}
	if !reflect.DeepEqual(info.Authors, wantAuthors) {
		t.Errorf("authors = %+v, want %+v", info.Authors, wantAuthors)
	}
	for _, mode := range []string{"--human --verbose", "--markdown --verbose"} {
		t.Run(mode, func(t *testing.T) {
			out := string(citationCLI(t, root, mode))
			for _, want := range []string{"Ada Lovelace", "Grace Hopper", "developer", "https://example.org/repo", "https://spdx.org/licenses/MIT", "Go", "science, metadata"} {
				if !strings.Contains(out, want) {
					t.Errorf("missing %q in output: %s", want, out)
				}
			}
		})
	}
}

func TestCodemetaCLIWrappedFields(t *testing.T) {
	root := t.TempDir()
	const content = `{
		"@context":"https://w3id.org/codemeta/3.0",
		"name":{"@value":"Example"},
		"description":{"@value":"Description","@language":null},
		"version":{"@set":["1.2","1.3"]},
		"author":[{"@set":[
			{"@type":"Person","givenName":{"@list":["Ada","Augusta"]},"familyName":{"@value":"Lovelace"}},
			{"@context":[{}],"schema:name":{"@value":"Grace Hopper"}},
			{"@type":"Role","roleName":{"@set":["creator","developer"]},"author":[{"@set":[{"name":"Team"}]}]}
		]}]
	}`
	if err := os.WriteFile(filepath.Join(root, "codemeta.json"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	var r brief.Report
	if err := json.Unmarshal(citationCLI(t, root, "--json"), &r); err != nil {
		t.Fatal(err)
	}
	if r.Resources == nil || r.Resources.Codemeta == nil {
		t.Fatal("missing CodeMeta")
	}
	info := r.Resources.Codemeta
	if info.ValidationStatus != "valid" || info.Name != "Example" || info.Description != "Description" || info.Version != "1.2, 1.3" {
		t.Fatalf("metadata: %+v", info)
	}
	want := []brief.CodemetaAuthor{
		{Name: "Ada Augusta Lovelace", GivenName: "Ada Augusta", FamilyName: "Lovelace", Kind: "person"},
		{Name: "Grace Hopper", Kind: "organization"},
		{Name: "Team", Role: "creator, developer", Kind: "organization"},
	}
	if !reflect.DeepEqual(info.Authors, want) {
		t.Fatalf("authors: %+v, want %+v", info.Authors, want)
	}
	for _, mode := range []string{"--human --verbose", "--markdown --verbose"} {
		out := string(citationCLI(t, root, mode))
		for _, value := range []string{"Example", "Description", "1.2, 1.3", "Ada Augusta Lovelace", "Grace Hopper", "Team", "creator, developer"} {
			if !strings.Contains(out, value) {
				t.Errorf("%s: missing %q in %s", mode, value, out)
			}
		}
	}
}

func TestCodemetaCLIAuthorIdentifiers(t *testing.T) {
	const identifier = "https://example.org/ada"
	for _, tc := range []struct {
		name, author, display string
		want                  brief.CodemetaAuthor
	}{
		{"reference", `{"@id":"` + identifier + `"}`, identifier,
			brief.CodemetaAuthor{Identifier: identifier, Kind: "reference"}},
		{"alias", `{"id":"` + identifier + `"}`, identifier,
			brief.CodemetaAuthor{Identifier: identifier, Kind: "reference"}},
		{"named person", `{"@type":"Person","@id":"` + identifier + `","name":"Ada"}`, "Ada",
			brief.CodemetaAuthor{Name: "Ada", Identifier: identifier, Kind: "person"}},
		{"role reference", `{"@type":"Role","roleName":"developer","author":{"@id":"` + identifier + `"}}`, identifier,
			brief.CodemetaAuthor{Identifier: identifier, Role: "developer", Kind: "reference"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			content := `{"@context":"https://w3id.org/codemeta/3.0","author":` + tc.author + `}`
			if err := os.WriteFile(filepath.Join(root, "codemeta.json"), []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			var r brief.Report
			if err := json.Unmarshal(citationCLI(t, root, "--json"), &r); err != nil {
				t.Fatal(err)
			}
			if r.Resources == nil || r.Resources.Codemeta == nil {
				t.Fatal("missing CodeMeta metadata")
			}
			info := r.Resources.Codemeta
			if info.ValidationStatus != "valid" || !reflect.DeepEqual(info.Authors, []brief.CodemetaAuthor{tc.want}) {
				t.Fatalf("metadata: %+v, want author %+v", info, tc.want)
			}
			for _, mode := range []string{"--human", "--markdown", "--human --verbose", "--markdown --verbose"} {
				checkCodemetaAuthorOutput(t, root, mode, tc.display, tc.want)
			}
		})
	}
}

func checkCodemetaAuthorOutput(t *testing.T, root, mode, display string, author brief.CodemetaAuthor) {
	t.Helper()
	out := string(citationCLI(t, root, mode))
	if !strings.Contains(strings.Join(strings.Fields(out), " "), "Authors: "+display) {
		t.Errorf("%s: missing author %q in %s", mode, display, out)
	}
	if author.Name != "" && strings.Contains(out, author.Identifier) {
		t.Errorf("%s: identifier displayed instead of name: %s", mode, out)
	}
	if author.Role != "" && strings.Contains(mode, "--verbose") && !strings.Contains(out, display+" "+author.Role) {
		t.Errorf("%s: missing author role in %s", mode, out)
	}
}

func TestCodemetaCLIValidationRegressions(t *testing.T) {
	for _, tc := range []struct{ name, fields, status string }{
		{"empty person field", `"author":{"@type":"Organization","name":"Team","givenName":null}`, "valid"},
		{"invalid reference", `"codeRepository":{"@id":"not an IRI"}`, "invalid"},
		{"null literal language", `"name":{"@value":"Example","@language":null}`, "valid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			content := `{"@context":"https://w3id.org/codemeta/3.0",` + tc.fields + `}`
			if err := os.WriteFile(filepath.Join(root, "codemeta.json"), []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			var r brief.Report
			if err := json.Unmarshal(citationCLI(t, root, "--json"), &r); err != nil {
				t.Fatal(err)
			}
			if r.Resources == nil || r.Resources.Codemeta == nil || r.Resources.Codemeta.ValidationStatus != tc.status {
				t.Fatalf("metadata: %+v", r.Resources)
			}
		})
	}
}
