package detect

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/git-pkgs/brief"
)

const minimalBibTeX = "@misc{k, title = {Example}, author = {Doe, Jane}}\n"

func TestBibTeXProject(t *testing.T) {
	t.Setenv("PATH", "")
	r, err := New(loadKB(t), "../testdata/bibtex-project").Run()
	if err != nil {
		t.Fatal(err)
	}
	res := r.Resources
	if res == nil || res.Citation != nil || res.Metadata["citation"] != "CITATION.bib" {
		t.Fatalf("resources: %+v", res)
	}
	info := res.BibTeX
	if info == nil || info.Path != "CITATION.bib" || info.ParseStatus != "parsed" || len(info.Entries) != 2 || len(info.Diagnostics) != 0 {
		t.Fatalf("bibtex: %+v", info)
	}
	sw := info.Entries[0]
	if sw.Type != "software" || sw.Key != "example_2024" || sw.Title != "Example: A GPU toolkit" || sw.Version != "v1.2.0" ||
		sw.DOI != "10.5281/zenodo.1234567" || sw.URL != "https://example.org/~ana/example" || sw.Year != "2024" || sw.Line != 4 {
		t.Fatalf("software entry: %+v", sw)
	}
	wantAuthors := []brief.CitationAuthor{
		{GivenNames: "Ana", FamilyNames: "Dvořák"},
		{GivenNames: "Jan", NameParticle: "van der", FamilyNames: "Berg"},
		{Name: "Example Research Consortium"},
	}
	if !reflect.DeepEqual(sw.Authors, wantAuthors) {
		t.Fatalf("authors: %+v", sw.Authors)
	}
	article := info.Entries[1]
	if article.Journal != "Journal of Open Source Software" || article.Pages != "5000–5004" || article.Volume != "8" || article.Number != "90" || len(article.Authors) != 2 {
		t.Fatalf("article entry: %+v", article)
	}
	readme := res.ReadmeBibTeX
	if readme == nil || readme.Path != "README.md" || len(readme.Entries) != 1 || readme.Entries[0].Booktitle != "Proceedings of the Example Conference" || readme.Entries[0].Eprint != "2201.01234" || readme.Entries[0].Line != 10 {
		t.Fatalf("readme bibtex: %+v", readme)
	}
}

func TestBibTeXCitationDiscovery(t *testing.T) {
	t.Setenv("PATH", "")
	for _, tc := range []struct {
		name, content string
		entries       int
	}{
		{"CITATION.bib", minimalBibTeX, 1},
		{"citation.BIB", minimalBibTeX, 1},
		{"CITATION.bib", "not bibtex at all", 0},
		{"CITATION", "Please cite:\n\n" + minimalBibTeX, 1},
		{"CITATION.md", "# Citing\n\n```bibtex\n" + minimalBibTeX + "```\n", 1},
		{"CITATION.txt", "Please cite our paper.", -1},
		{"CITATION.cff", minimalBibTeX, -1},
	} {
		t.Run(tc.name+" "+tc.content[:8], func(t *testing.T) {
			r := scanCitation(t, tc.name, tc.content)
			info := r.Resources.BibTeX
			if tc.entries < 0 {
				if info != nil {
					t.Fatalf("unexpected bibtex: %+v", info)
				}
				return
			}
			if info == nil || info.Path != tc.name || len(info.Entries) != tc.entries || r.Resources.Metadata["citation"] != tc.name {
				t.Fatalf("bibtex: %+v", info)
			}
		})
	}
}

func TestBibTeXBesideOtherCitationFiles(t *testing.T) {
	t.Setenv("PATH", "")
	for _, name := range []string{"CITATION.cff", "CITATION"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, name, "Please cite our software.\n")
			writeFile(t, dir, "citation.BIB", minimalBibTeX)
			r, err := New(loadKB(t), dir).Run()
			if err != nil {
				t.Fatal(err)
			}
			info := r.Resources.BibTeX
			if info == nil || info.Path != "citation.BIB" || len(info.Entries) != 1 {
				t.Fatalf("bibtex: %+v", info)
			}
			if r.Resources.Metadata["citation"] != name {
				t.Fatalf("citation path: %v", r.Resources.Metadata)
			}
			if name == "CITATION.cff" && r.Resources.Citation == nil {
				t.Fatal("CFF result missing")
			}
		})
	}
}

func TestBibTeXExtractionErrors(t *testing.T) {
	t.Setenv("PATH", "")
	for _, name := range []string{"CITATION.md", "README.md"} {
		for _, tc := range []struct {
			name, content, status, code string
		}{
			{"oversized", minimalBibTeX + strings.Repeat("\n", bibtexByteLimit), "limit_exceeded", "byte_limit"},
			{"UTF-16", "\xff\xfe@\x00", "unsupported_encoding", "encoding"},
		} {
			t.Run(name+"/"+tc.name, func(t *testing.T) {
				dir := t.TempDir()
				writeFile(t, dir, name, tc.content)
				r, err := New(loadKB(t), dir).Run()
				if err != nil {
					t.Fatal(err)
				}
				info := r.Resources.BibTeX
				if name == "README.md" {
					info = r.Resources.ReadmeBibTeX
				}
				if info == nil || info.ParseStatus != tc.status || len(info.Diagnostics) != 1 || info.Diagnostics[0].Code != tc.code {
					t.Fatalf("outcome: %+v", info)
				}
				changed := FilterByChangedFiles(r, loadKB(t), []string{name})
				if changed.Resources == nil || changed.Resources.BibTeX == nil && changed.Resources.ReadmeBibTeX == nil {
					t.Fatal("diff omitted extraction error")
				}
			})
		}
	}
}

func TestBibTeXOutcomes(t *testing.T) {
	t.Setenv("PATH", "")
	for _, tc := range []struct {
		name, content, status, code string
	}{
		{"diagnostics", "@misc{k\n title = {T}}", "parsed", "missing_comma"},
		{"windows-1252", "@misc{k, title = {M\xfcller}}", "parsed", ""},
		{"utf-16", "\xff\xfe@\x00", "unsupported_encoding", "encoding"},
		{"exact limit", minimalBibTeX + strings.Repeat("\n", bibtexByteLimit-len(minimalBibTeX)), "parsed", ""},
		{"over limit", minimalBibTeX + strings.Repeat("\n", bibtexByteLimit+1-len(minimalBibTeX)), "limit_exceeded", "byte_limit"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			info := scanCitation(t, "CITATION.bib", tc.content).Resources.BibTeX
			if info == nil || info.ParseStatus != tc.status {
				t.Fatalf("outcome: %+v", info)
			}
			if tc.code != "" && (len(info.Diagnostics) == 0 || info.Diagnostics[0].Code != tc.code) {
				t.Fatalf("diagnostics: %+v", info.Diagnostics)
			}
		})
	}
	info := scanCitation(t, "CITATION.bib", "@misc{k, title = {M\xfcller}}").Resources.BibTeX
	if info.Encoding != "windows-1252" || info.Entries[0].Title != "Müller" {
		t.Fatalf("encoding: %+v", info)
	}
}

func TestReadmeBibTeX(t *testing.T) {
	t.Setenv("PATH", "")
	for _, tc := range []struct {
		name, content string
		entries       int
	}{
		{"README.md", "# X\n\n```bibtex\n" + minimalBibTeX + "```\n", 1},
		{"README.rst", "Cite\n----\n\n.. code:: bibtex\n\n    " + minimalBibTeX, 1},
		{"README.md", "# X\n\n```python\n@dataclass(frozen=True)\nclass C: pass\n```\n", 0},
		{"README.md", "# X\n\nNo citation here.\n", 0},
	} {
		dir := t.TempDir()
		writeFile(t, dir, tc.name, tc.content)
		r, err := New(loadKB(t), dir).Run()
		if err != nil {
			t.Fatal(err)
		}
		info := r.Resources.ReadmeBibTeX
		if tc.entries == 0 {
			if info != nil {
				t.Fatalf("%q: unexpected readme bibtex %+v", tc.content, info)
			}
			continue
		}
		if info == nil || info.Path != tc.name || len(info.Entries) != tc.entries || info.Entries[0].Authors[0].FamilyNames != "Doe" {
			t.Fatalf("%q: readme bibtex %+v", tc.content, info)
		}
	}
}

func TestBibTeXDiffFilter(t *testing.T) {
	t.Setenv("PATH", "")
	r, err := New(loadKB(t), "../testdata/bibtex-project").Run()
	if err != nil {
		t.Fatal(err)
	}
	kb := loadKB(t)
	changed := FilterByChangedFiles(r, kb, []string{"CITATION.bib"})
	if changed.Resources == nil || changed.Resources.BibTeX == nil || changed.Resources.ReadmeBibTeX != nil {
		t.Fatalf("CITATION.bib change: %+v", changed.Resources)
	}
	changed = FilterByChangedFiles(r, kb, []string{"README.md"})
	if changed.Resources == nil || changed.Resources.ReadmeBibTeX == nil || changed.Resources.BibTeX != nil {
		t.Fatalf("README change: %+v", changed.Resources)
	}
	unrelated := FilterByChangedFiles(r, kb, []string{"main.go"})
	if unrelated.Resources != nil && (unrelated.Resources.BibTeX != nil || unrelated.Resources.ReadmeBibTeX != nil) {
		t.Fatal("unrelated change retained bibtex")
	}
}

func TestBibTeXSymlinkEscape(t *testing.T) {
	t.Setenv("PATH", "")
	dir := t.TempDir()
	target := filepath.Join(t.TempDir(), "private.bib")
	if err := os.WriteFile(target, []byte(minimalBibTeX), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"CITATION.bib", "README.md"} {
		if err := os.Symlink(target, filepath.Join(dir, name)); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}
	}
	r, err := New(loadKB(t), dir).Run()
	if err != nil {
		t.Fatal(err)
	}
	if r.Resources == nil {
		return
	}
	if info := r.Resources.BibTeX; info != nil && (info.ParseStatus != "read_error" || len(info.Entries) != 0) {
		t.Fatalf("escaped root: %+v", info)
	}
	if info := r.Resources.ReadmeBibTeX; info != nil && (info.ParseStatus != "read_error" || len(info.Entries) != 0) {
		t.Fatalf("escaped root via README: %+v", info)
	}
}
