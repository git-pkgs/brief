package report

import (
	"fmt"
	"io"
	"strings"

	"github.com/git-pkgs/brief"
)

func printCodemeta(w io.Writer, info *brief.CodemetaInfo, verbose bool) {
	for _, row := range codemetaRows(info, verbose) {
		_, _ = fmt.Fprintf(w, "%-12s %s\n", row.label+":", citationText(row.value))
	}
}

func mdCodemeta(w io.Writer, info *brief.CodemetaInfo, verbose bool) {
	rows := codemetaRows(info, verbose)
	if len(rows) == 0 {
		return
	}
	_, _ = fmt.Fprint(w, "\n**CodeMeta:**\n\n")
	for _, row := range rows {
		_, _ = fmt.Fprintf(w, "- %s: %s\n", row.label, escapeCitationMarkdown(citationText(row.value)))
	}
}

func codemetaRows(info *brief.CodemetaInfo, verbose bool) []citationRow {
	if info == nil {
		return nil
	}
	var rows []citationRow
	add := func(label, value string) {
		if value != "" {
			rows = append(rows, citationRow{label, value})
		}
	}
	add("Software", info.Name)
	add("Authors", codemetaAuthorSummary(info.Authors))
	add("Version", info.Version)
	status := info.ParseStatus
	if info.ValidationStatus != "" {
		status = strings.TrimSpace(info.ContextVersion + " " + info.ValidationStatus)
	}
	add("CodeMeta", status)
	for _, issue := range info.Diagnostics[:min(len(info.Diagnostics), citationAuthorLimit)] {
		location := diagnosticLocation(info.Path, issue.Path, issue.Line, issue.Column)
		add("CodeMeta issue", strings.TrimSpace(location+" "+issue.Code+": "+issue.Message))
	}
	if verbose {
		add("Description", info.Description)
		add("Code", strings.Join(info.CodeRepository, ", "))
		add("Licenses", strings.Join(info.Licenses, ", "))
		add("Keywords", strings.Join(info.Keywords, ", "))
		add("Languages", strings.Join(info.ProgrammingLanguages, ", "))
		for _, author := range info.Authors[:min(len(info.Authors), maxDisplayItems)] {
			if author.Role != "" {
				add("Author role", strings.TrimSpace(author.Name+" "+author.Role))
			}
		}
	}
	return rows
}

func codemetaAuthorSummary(authors []brief.CodemetaAuthor) string {
	var names []string
	for _, author := range authors[:min(len(authors), citationAuthorLimit)] {
		if author.Name != "" {
			names = append(names, author.Name)
		}
	}
	if len(authors) > citationAuthorLimit {
		names = append(names, fmt.Sprintf("and %d more", len(authors)-citationAuthorLimit))
	}
	return strings.Join(names, ", ")
}
