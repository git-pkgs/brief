package detect

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/git-pkgs/brief"
)

const minimalCitation = "cff-version: 1.2.0\nmessage: Cite\ntitle: Example\nauthors: [{name: Team}]\n"

func TestCitationMetadata(t *testing.T) {
	t.Setenv("PATH", "")
	r, err := New(loadKB(t), "../testdata/citation-project").Run()
	if err != nil {
		t.Fatal(err)
	}
	info := r.Resources.Citation
	if info == nil || info.ParseStatus != "parsed" || info.ValidationStatus != "valid" {
		t.Fatalf("citation: %+v", info)
	}
	if info.Path != "CITATION.cff" || r.Resources.Metadata["citation"] != info.Path || info.CFFVersion != "1.2.0" {
		t.Fatalf("provenance: %+v", info)
	}
	if info.Title != "Example Research Software" || info.Type != "software" || info.Version != "1.20" || info.DateReleased != "2024-02-29" {
		t.Fatalf("release: %+v", info)
	}
	if len(info.Authors) != 2 || info.Authors[0].NameParticle != "van" || info.Authors[0].NameSuffix != "III" || info.Authors[0].Affiliation != "Example Institute" || info.Authors[0].ORCID == "" || info.Authors[1].Name != "Research Team" {
		t.Fatalf("authors: %+v", info.Authors)
	}
	if !reflect.DeepEqual(info.Licenses, []string{"MIT", "Apache-2.0"}) || !reflect.DeepEqual(info.Keywords, []string{"simulation", "research"}) {
		t.Fatalf("lists: %+v", info)
	}
	if info.Abstract != "Simulates example systems for research." || info.Message != "Please cite the accompanying paper." || info.URL != "https://example.org" || info.RepositoryCode == "" || info.RepositoryArtifact == "" {
		t.Fatalf("text metadata: %+v", info)
	}
	if info.DOI != "10.1234/software" || len(info.Identifiers) != 1 || info.Identifiers[0].Value != "10.1234/archive" {
		t.Fatalf("identifiers: %+v", info)
	}
	checkPreferredCitation(t, info.PreferredCitation)
}

func checkPreferredCitation(t *testing.T, ref *brief.CitationReference) {
	t.Helper()
	if ref == nil || ref.DOI != "10.1234/paper" || ref.Title != "The accompanying paper" || ref.Type != "article" || ref.Year != "2024" || ref.Volume != "2" || ref.Issue != "1" || ref.Publisher == nil || ref.Publisher.Name != "Example Press" || ref.Start != "10" || ref.End != "20" {
		t.Fatalf("preferred citation: %+v", ref)
	}
}

func TestCitationOutcomes(t *testing.T) {
	t.Setenv("PATH", "")
	for _, tc := range []struct {
		name, content, parse, validation, code string
	}{
		{"valid", minimalCitation, "parsed", "valid", ""},
		{"malformed", "authors: [", "syntax_error", "", "flow_end"},
		{"wrong root", "[a, b]", "type_error", "", "root_type"},
		{"invalid", "cff-version: 1.2.0\ntitle: Example\n", "parsed", "invalid", "required"},
		{"historical", strings.Replace(minimalCitation, "1.2.0", "1.0.3", 1), "parsed", "unsupported_version", "unsupported_version"},
		{"missing version", "title: Example\n", "parsed", "invalid", "required"},
		{"unsupported syntax", "title: !custom Example\n", "unsupported_syntax", "", "tag"},
		{"exact limit", minimalCitation + strings.Repeat("\n", citationByteLimit-len(minimalCitation)), "parsed", "valid", ""},
		{"over limit", minimalCitation + strings.Repeat("\n", citationByteLimit+1-len(minimalCitation)), "limit_exceeded", "", "byte_limit"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := scanCitation(t, "CITATION.cff", tc.content)
			info := r.Resources.Citation
			if info == nil || info.ParseStatus != tc.parse || info.ValidationStatus != tc.validation {
				t.Fatalf("outcome: %+v", info)
			}
			if tc.code != "" && (len(info.Diagnostics) == 0 || info.Diagnostics[0].Code != tc.code) {
				t.Fatalf("diagnostics: %+v", info.Diagnostics)
			}
			if r.Resources.Metadata["citation"] != "CITATION.cff" {
				t.Fatal("source path lost")
			}
		})
	}
}

func TestCitationDiscoveryAndProjection(t *testing.T) {
	t.Setenv("PATH", "")
	for _, name := range []string{"citation.CFF", "CITATION", "CITATION.md", "CITATION.bib", "CITATION.txt"} {
		r := scanCitation(t, name, minimalCitation)
		wantCFF := strings.EqualFold(filepath.Ext(name), ".cff")
		if (r.Resources.Citation != nil) != wantCFF || r.Resources.Metadata["citation"] != name {
			t.Fatalf("%s: %+v", name, r.Resources)
		}
	}
	r := scanCitation(t, "CITATION.cff", "cff-version: 1.2.0\ntitle: true\nversion: false\nlicense: MIT\nkeywords: [fine, 12]\nauthors: [false, {name: 12}]\n")
	info := r.Resources.Citation
	if info.Title != "" || info.Version != "" || len(info.Authors) != 1 || info.Authors[0].Name != "" || !reflect.DeepEqual(info.Keywords, []string{"fine"}) || !reflect.DeepEqual(info.Licenses, []string{"MIT"}) || info.ValidationStatus != "invalid" {
		t.Fatalf("invalid values converted to metadata: %+v", info)
	}
}

func TestCitationDiffFilter(t *testing.T) {
	t.Setenv("PATH", "")
	r := scanCitation(t, "CITATION.cff", minimalCitation)
	kb := loadKB(t)
	changed := FilterByChangedFiles(r, kb, []string{"CITATION.cff"})
	if changed.Resources == nil || changed.Resources.Citation == nil || changed.Resources.Citation.Title != "Example" {
		t.Fatal("changed citation omitted")
	}
	unrelated := FilterByChangedFiles(r, kb, []string{"main.go"})
	if unrelated.Resources != nil && unrelated.Resources.Citation != nil {
		t.Fatal("unrelated change retained citation")
	}
}

func TestCitationSymlinkEscape(t *testing.T) {
	t.Setenv("PATH", "")
	dir := t.TempDir()
	target := filepath.Join(t.TempDir(), "private.cff")
	if err := os.WriteFile(target, []byte(minimalCitation), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, "CITATION.cff")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	r, err := New(loadKB(t), dir).Run()
	if err != nil {
		t.Fatal(err)
	}
	if r.Resources != nil && r.Resources.Citation != nil {
		info := r.Resources.Citation
		if info.ParseStatus != "read_error" || info.Title != "" {
			t.Fatalf("escaped root: %+v", info)
		}
	}
}

func scanCitation(t *testing.T, name, content string) *brief.Report {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, dir, name, content)
	r, err := New(loadKB(t), dir).Run()
	if err != nil {
		t.Fatal(err)
	}
	if r.Resources == nil {
		t.Fatal("missing resources")
	}
	return r
}
