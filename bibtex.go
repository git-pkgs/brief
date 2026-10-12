package brief

// BibTeXInfo is the BibTeX found in a citation file such as CITATION.bib, or
// embedded in a README. Field values are converted to plain text.
type BibTeXInfo struct {
	Path        string `json:"path"`
	ParseStatus string `json:"parse_status"`
	// Encoding is set when the file was not UTF-8 and was decoded as
	// Windows-1252.
	Encoding    string               `json:"encoding,omitempty"`
	Entries     []BibTeXEntry        `json:"entries,omitempty"`
	Diagnostics []CitationDiagnostic `json:"diagnostics,omitempty"`
}

// BibTeXEntry is one BibTeX entry such as @article{key, ...}.
type BibTeXEntry struct {
	Type      string           `json:"type"`
	Key       string           `json:"key,omitempty"`
	Title     string           `json:"title,omitempty"`
	Authors   []CitationAuthor `json:"authors,omitempty"`
	Year      string           `json:"year,omitempty"`
	DOI       string           `json:"doi,omitempty"`
	URL       string           `json:"url,omitempty"`
	Version   string           `json:"version,omitempty"`
	Journal   string           `json:"journal,omitempty"`
	Booktitle string           `json:"booktitle,omitempty"`
	Publisher string           `json:"publisher,omitempty"`
	Volume    string           `json:"volume,omitempty"`
	Number    string           `json:"number,omitempty"`
	Pages     string           `json:"pages,omitempty"`
	Eprint    string           `json:"eprint,omitempty"`
	Line      int              `json:"line,omitempty"`
}
