package detect

import (
	"errors"
	"path/filepath"
	"strings"

	"github.com/git-pkgs/bibtex"
	"github.com/git-pkgs/brief"
)

const (
	bibtexByteLimit     = 1 << 20
	bibtexReadError     = "read_error"
	bibtexLimitExceeded = "limit_exceeded"
	bibtexParsed        = "parsed"
)

// detectBibTeX parses a .bib file, or with extract set finds entries embedded
// in a document such as a README or a CITATION file in another format.
func (e *Engine) detectBibTeX(path string, extract bool) *brief.BibTeXInfo {
	info := &brief.BibTeXInfo{Path: path}
	data, err := e.safeReadFileLimit(path, bibtexByteLimit+1)
	if err != nil {
		info.ParseStatus = bibtexReadError
		info.Diagnostics = []brief.CitationDiagnostic{{Code: bibtexReadError, Message: err.Error()}}
		return info
	}
	opts := bibtex.ParseOptions{MaxBytes: bibtexByteLimit}
	var f *bibtex.File
	if extract {
		f, err = bibtex.Extract(data, opts)
	} else {
		f, err = bibtex.ParseWithOptions(data, opts)
	}
	if err != nil {
		switch {
		case errors.Is(err, bibtex.ErrLimit):
			info.ParseStatus = bibtexLimitExceeded
		case errors.Is(err, bibtex.ErrUnsupported):
			info.ParseStatus = "unsupported_encoding"
		default:
			info.ParseStatus = "parse_error"
		}
		var problem *bibtex.Error
		if errors.As(err, &problem) {
			info.Diagnostics = []brief.CitationDiagnostic{bibtexDiagnostic(problem.Diagnostic)}
		}
		return info
	}
	info.ParseStatus = bibtexParsed
	if f.Encoding != "utf-8" {
		info.Encoding = f.Encoding
	}
	for i := range f.Entries {
		info.Entries = append(info.Entries, bibtexEntry(&f.Entries[i]))
	}
	for _, d := range f.Diagnostics {
		info.Diagnostics = append(info.Diagnostics, bibtexDiagnostic(d))
	}
	return info
}

func bibtexEntry(e *bibtex.Entry) brief.BibTeXEntry {
	out := brief.BibTeXEntry{
		Type: e.Type, Key: e.Key, Title: e.Text("title"), Year: e.Text("year"),
		DOI: e.Text("doi"), URL: e.Text("url"), Version: e.Text("version"),
		Journal: e.Text("journal"), Booktitle: e.Text("booktitle"), Publisher: e.Text("publisher"),
		Volume: e.Text("volume"), Number: e.Text("number"), Pages: e.Text("pages"),
		Eprint: e.Text("eprint"), Line: e.Line,
	}
	for _, n := range e.Authors() {
		switch {
		case n.Others:
		case n.Literal:
			out.Authors = append(out.Authors, brief.CitationAuthor{Name: n.Last})
		default:
			out.Authors = append(out.Authors, brief.CitationAuthor{
				GivenNames: n.First, NameParticle: n.Von, FamilyNames: n.Last, NameSuffix: n.Jr,
			})
		}
	}
	return out
}

func bibtexDiagnostic(d bibtex.Diagnostic) brief.CitationDiagnostic {
	return brief.CitationDiagnostic{Code: d.Code, Message: d.Message, Line: d.Line, Column: d.Column}
}

// detectCitationBibTeX reads a citation file that is not CFF. A .bib file is
// parsed as BibTeX; CITATION, CITATION.md and similar files often hold a
// BibTeX entry among prose, so empty results without diagnostics are omitted.
func (e *Engine) detectCitationBibTeX(path string) *brief.BibTeXInfo {
	if strings.EqualFold(filepath.Ext(path), ".bib") {
		return e.detectBibTeX(path, false)
	}
	if info := e.detectBibTeX(path, true); bibtexReportable(info) {
		return info
	}
	return nil
}

func bibtexReportable(info *brief.BibTeXInfo) bool {
	return info.ParseStatus != bibtexParsed || len(info.Entries) > 0 || len(info.Diagnostics) > 0
}

func (e *Engine) detectCitationBibTeXFiles() *brief.BibTeXInfo {
	for _, rd := range e.KB.Resources {
		if rd.Resource.Group != "metadata" || rd.Resource.Field != "citation" {
			continue
		}
		for _, pattern := range rd.Resource.Patterns {
			if strings.EqualFold(filepath.Ext(pattern), ".cff") {
				continue
			}
			resource := rd.Resource
			resource.Patterns = []string{pattern}
			_, rel := e.findResource(resource)
			if rel == "" {
				continue
			}
			if info := e.detectCitationBibTeX(rel); info != nil {
				return info
			}
		}
	}
	return nil
}
