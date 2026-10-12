package report

import (
	"fmt"
	"io"
	"strings"

	"github.com/git-pkgs/brief"
)

func printBibTeX(w io.Writer, info *brief.BibTeXInfo, verbose bool) {
	for _, row := range bibtexRows(info, verbose) {
		_, _ = fmt.Fprintf(w, "%-12s %s\n", row.label+":", row.value)
	}
}

func mdBibTeX(w io.Writer, info *brief.BibTeXInfo, verbose bool) {
	rows := bibtexRows(info, verbose)
	if len(rows) == 0 {
		return
	}
	_, _ = fmt.Fprint(w, "\n**BibTeX:**\n\n")
	for _, row := range rows {
		_, _ = fmt.Fprintf(w, "- %s: %s\n", row.label, escapeCitationMarkdown(row.value))
	}
}

// bibtexRows summarises a BibTeX source. Without verbose only the first
// entry is detailed.
func bibtexRows(info *brief.BibTeXInfo, verbose bool) []citationRow {
	if info == nil {
		return nil
	}
	var rows []citationRow
	add := func(label, value string) {
		if value != "" {
			rows = append(rows, citationRow{label, citationText(value)})
		}
	}
	summary := info.Path + " " + info.ParseStatus
	if info.ParseStatus == "parsed" {
		summary = fmt.Sprintf("%s, %d %s", info.Path, len(info.Entries), plural(len(info.Entries), "entry", "entries"))
	}
	add("BibTeX", summary)
	shown := info.Entries
	if !verbose {
		shown = shown[:min(len(shown), 1)]
	}
	for _, e := range shown {
		title := e.Title
		if title == "" {
			title = e.Type + " " + e.Key
		}
		add("Cite", title)
		add("Authors", citationAuthorSummary(e.Authors))
		add("Published", strings.TrimSpace(firstNonEmpty(e.Journal, e.Booktitle, e.Publisher)+" "+e.Year))
		add("DOI", e.DOI)
		if verbose {
			add("Type", strings.TrimSpace(e.Type+" "+e.Key))
			add("Version", e.Version)
			add("URL", e.URL)
			add("Eprint", e.Eprint)
		}
	}
	for _, issue := range info.Diagnostics[:min(len(info.Diagnostics), citationAuthorLimit)] {
		location := diagnosticLocation(info.Path, "", issue.Line, issue.Column)
		add("BibTeX issue", strings.TrimSpace(location+" "+issue.Code+": "+issue.Message))
	}
	return rows
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
