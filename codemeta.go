package brief

// CodemetaInfo contains declared software metadata and its parse and validation outcomes.
type CodemetaInfo struct {
	Path                 string               `json:"path"`
	ParseStatus          string               `json:"parse_status"`
	ValidationStatus     string               `json:"validation_status,omitempty"`
	ContextVersion       string               `json:"context_version,omitempty"`
	Name                 string               `json:"name,omitempty"`
	Description          string               `json:"description,omitempty"`
	Version              string               `json:"version,omitempty"`
	CodeRepository       []string             `json:"code_repository,omitempty"`
	Licenses             []string             `json:"licenses,omitempty"`
	Keywords             []string             `json:"keywords,omitempty"`
	ProgrammingLanguages []string             `json:"programming_languages,omitempty"`
	Authors              []CodemetaAuthor     `json:"authors,omitempty"`
	Diagnostics          []CodemetaDiagnostic `json:"diagnostics,omitempty"`
}

// CodemetaAuthor is a person, organisation, or role declared as an author.
type CodemetaAuthor struct {
	Name       string `json:"name,omitempty"`
	GivenName  string `json:"given_name,omitempty"`
	FamilyName string `json:"family_name,omitempty"`
	Role       string `json:"role,omitempty"`
	Kind       string `json:"kind,omitempty"`
}

// CodemetaDiagnostic identifies a read, parse, or validation problem in codemeta.json.
type CodemetaDiagnostic struct {
	Code    string `json:"code"`
	Path    string `json:"path,omitempty"`
	Message string `json:"message"`
	Line    int    `json:"line,omitempty"`
	Column  int    `json:"column,omitempty"`
}
