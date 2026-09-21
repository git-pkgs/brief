package detect

import (
	"errors"

	"github.com/git-pkgs/brief"
	"github.com/git-pkgs/citation"
)

const citationByteLimit = 1 << 20

func (e *Engine) detectCitation(path string) *brief.CitationInfo {
	info := &brief.CitationInfo{Path: path}
	data, err := e.safeReadFileLimit(path, citationByteLimit+1)
	if err != nil {
		info.ParseStatus = "read_error"
		info.Diagnostics = []brief.CitationDiagnostic{{Code: "read_error", Message: err.Error()}}
		return info
	}
	doc, err := citation.ParseWithOptions(data, citation.ParseOptions{MaxBytes: citationByteLimit})
	if err != nil {
		info.ParseStatus = "syntax_error"
		switch {
		case errors.Is(err, citation.ErrLimit):
			info.ParseStatus = "limit_exceeded"
		case errors.Is(err, citation.ErrUnsupported):
			info.ParseStatus = "unsupported_syntax"
		case errors.Is(err, citation.ErrType):
			info.ParseStatus = "type_error"
		}
		var problem *citation.Error
		if errors.As(err, &problem) {
			info.Diagnostics = []brief.CitationDiagnostic{citationDiagnostic(problem.Diagnostic)}
		}
		return info
	}
	info.ParseStatus = "parsed"
	info.ValidationStatus = "valid"
	for _, issue := range doc.Validate() {
		info.ValidationStatus = "invalid"
		if issue.Code == "unsupported_version" {
			info.ValidationStatus = "unsupported_version"
		}
		info.Diagnostics = append(info.Diagnostics, citationDiagnostic(issue))
	}
	populateCitation(info, doc)
	return info
}

func citationDiagnostic(issue citation.Diagnostic) brief.CitationDiagnostic {
	return brief.CitationDiagnostic{Code: issue.Code, Path: issue.Path, Message: issue.Message, Line: issue.Line, Column: issue.Column}
}

func populateCitation(info *brief.CitationInfo, doc *citation.Document) {
	info.CFFVersion = citationString(doc.Get("cff-version"))
	info.Title = citationString(doc.Get("title"))
	info.Type = citationString(doc.Get("type"))
	info.Version = citationScalar(doc.Get("version"))
	info.DateReleased = citationString(doc.Get("date-released"))
	info.DOI = citationString(doc.Get("doi"))
	info.URL = citationString(doc.Get("url"))
	info.Repository = citationString(doc.Get("repository"))
	info.RepositoryCode = citationString(doc.Get("repository-code"))
	info.RepositoryArtifact = citationString(doc.Get("repository-artifact"))
	info.Authors = citationAuthors(doc.Get("authors"))
	info.Identifiers = citationIdentifiers(doc.Get("identifiers"))
	info.Licenses = citationStrings(doc.Get("license"))
	if license := citationString(doc.Get("license")); license != "" {
		info.Licenses = []string{license}
	}
	info.Keywords = citationStrings(doc.Get("keywords"))
	info.Abstract = citationString(doc.Get("abstract"))
	info.Message = citationString(doc.Get("message"))
	if value := doc.Get("preferred-citation"); value.Kind() == citation.Mapping {
		info.PreferredCitation = citationReference(value)
	}
}

func citationString(v citation.Value) string {
	if v.Kind() == citation.String {
		return v.Text()
	}
	return ""
}

func citationScalar(v citation.Value) string {
	if v.Kind() == citation.Number {
		return v.Text()
	}
	return citationString(v)
}

func citationStrings(v citation.Value) []string {
	var values []string
	for _, item := range v.Items() {
		if item.Kind() == citation.String {
			values = append(values, item.Text())
		}
	}
	return values
}

func citationAuthor(v citation.Value) brief.CitationAuthor {
	return brief.CitationAuthor{
		Name: citationString(v.Get("name")), GivenNames: citationString(v.Get("given-names")),
		FamilyNames: citationString(v.Get("family-names")), NameParticle: citationString(v.Get("name-particle")),
		NameSuffix: citationString(v.Get("name-suffix")), Affiliation: citationString(v.Get("affiliation")),
		ORCID: citationString(v.Get("orcid")),
	}
}

func citationAuthors(v citation.Value) []brief.CitationAuthor {
	var authors []brief.CitationAuthor
	for _, item := range v.Items() {
		if item.Kind() == citation.Mapping {
			authors = append(authors, citationAuthor(item))
		}
	}
	return authors
}

func citationIdentifiers(v citation.Value) []brief.CitationIdentifier {
	var identifiers []brief.CitationIdentifier
	for _, item := range v.Items() {
		if item.Kind() == citation.Mapping {
			identifiers = append(identifiers, brief.CitationIdentifier{
				Type: citationString(item.Get("type")), Value: citationString(item.Get("value")),
				Description: citationString(item.Get("description")),
			})
		}
	}
	return identifiers
}

func citationReference(v citation.Value) *brief.CitationReference {
	ref := &brief.CitationReference{
		Title: citationString(v.Get("title")), Type: citationString(v.Get("type")),
		Authors: citationAuthors(v.Get("authors")), DOI: citationString(v.Get("doi")),
		URL: citationString(v.Get("url")), Identifiers: citationIdentifiers(v.Get("identifiers")),
		DatePublished: citationString(v.Get("date-published")), Year: citationScalar(v.Get("year")),
		Journal: citationString(v.Get("journal")), Volume: citationScalar(v.Get("volume")),
		Issue: citationScalar(v.Get("issue")), Start: citationScalar(v.Get("start")),
		End: citationScalar(v.Get("end")), Pages: citationScalar(v.Get("pages")),
	}
	if publisher := v.Get("publisher"); publisher.Kind() == citation.Mapping {
		value := citationAuthor(publisher)
		ref.Publisher = &value
	}
	return ref
}
