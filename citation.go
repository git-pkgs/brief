package brief

// CitationInfo contains declared metadata and independent parsing and validation outcomes.
type CitationInfo struct {
	Path               string               `json:"path"`
	ParseStatus        string               `json:"parse_status"`
	ValidationStatus   string               `json:"validation_status,omitempty"`
	CFFVersion         string               `json:"cff_version,omitempty"`
	Title              string               `json:"title,omitempty"`
	Type               string               `json:"type,omitempty"`
	Version            string               `json:"version,omitempty"`
	DateReleased       string               `json:"date_released,omitempty"`
	DOI                string               `json:"doi,omitempty"`
	URL                string               `json:"url,omitempty"`
	Repository         string               `json:"repository,omitempty"`
	RepositoryCode     string               `json:"repository_code,omitempty"`
	RepositoryArtifact string               `json:"repository_artifact,omitempty"`
	Authors            []CitationAuthor     `json:"authors,omitempty"`
	Identifiers        []CitationIdentifier `json:"identifiers,omitempty"`
	Licenses           []string             `json:"licenses,omitempty"`
	Keywords           []string             `json:"keywords,omitempty"`
	Abstract           string               `json:"abstract,omitempty"`
	Message            string               `json:"message,omitempty"`
	PreferredCitation  *CitationReference   `json:"preferred_citation,omitempty"`
	Diagnostics        []CitationDiagnostic `json:"diagnostics,omitempty"`
}

// CitationAuthor preserves person and organisation name fields as declared.
type CitationAuthor struct {
	Name         string `json:"name,omitempty"`
	GivenNames   string `json:"given_names,omitempty"`
	FamilyNames  string `json:"family_names,omitempty"`
	NameParticle string `json:"name_particle,omitempty"`
	NameSuffix   string `json:"name_suffix,omitempty"`
	Affiliation  string `json:"affiliation,omitempty"`
	ORCID        string `json:"orcid,omitempty"`
}

// CitationIdentifier is an identifier declared in the CFF identifiers list.
type CitationIdentifier struct {
	Type        string `json:"type,omitempty"`
	Value       string `json:"value,omitempty"`
	Description string `json:"description,omitempty"`
}

// CitationReference describes the work the project asks users to cite.
type CitationReference struct {
	Title         string               `json:"title,omitempty"`
	Type          string               `json:"type,omitempty"`
	Authors       []CitationAuthor     `json:"authors,omitempty"`
	DOI           string               `json:"doi,omitempty"`
	URL           string               `json:"url,omitempty"`
	Identifiers   []CitationIdentifier `json:"identifiers,omitempty"`
	DatePublished string               `json:"date_published,omitempty"`
	Year          string               `json:"year,omitempty"`
	Journal       string               `json:"journal,omitempty"`
	Volume        string               `json:"volume,omitempty"`
	Issue         string               `json:"issue,omitempty"`
	Start         string               `json:"start,omitempty"`
	End           string               `json:"end,omitempty"`
	Pages         string               `json:"pages,omitempty"`
	Publisher     *CitationAuthor      `json:"publisher,omitempty"`
}

// CitationDiagnostic identifies a read, parse, or validation problem in the source file.
type CitationDiagnostic struct {
	Code    string `json:"code"`
	Path    string `json:"path,omitempty"`
	Message string `json:"message"`
	Line    int    `json:"line,omitempty"`
	Column  int    `json:"column,omitempty"`
}
