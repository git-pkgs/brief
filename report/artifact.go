package report

import (
	"fmt"
	"io"

	"github.com/git-pkgs/brief"
)

// ArtifactMarkdown writes the artifact report in Markdown format.
func ArtifactMarkdown(w io.Writer, a *brief.Artifact) {
	text := func(value string) string { return escapeCitationMarkdown(sanitizeLine(value)) }
	field := func(label, value string) {
		if value != "" {
			_, _ = fmt.Fprintf(w, "**%s:** %s\n\n", label, text(value))
		}
	}
	_, _ = fmt.Fprintf(w, "# brief %s — %s\n\n", text(a.Version), text(a.Path))
	field("Format", a.Format)
	field("SHA256", a.SHA256)
	_, _ = fmt.Fprintf(w, "**Entries:** %d files, %d native objects\n\n", a.Entries, len(a.NativeObjects))

	for _, obj := range a.NativeObjects {
		_, _ = fmt.Fprintf(w, "## Object: %s\n\n", text(obj.Path))
		field("Format", obj.Format)
		field("Architecture", obj.Arch)
		field("SOName", obj.SOName)
		for _, producer := range obj.Producer {
			field("Producer", producer)
		}
		for _, needed := range obj.Needed {
			field("Needed", needed)
		}
		if obj.Go != nil {
			field("Go", obj.Go.Version)
			field("Go path", obj.Go.Path)
			field("Go main", obj.Go.Main)
			for _, dep := range obj.Go.Deps {
				field("Go dependency", dep)
			}
		}
		for _, hint := range obj.Static {
			label := hint.Library
			if hint.Version != "" {
				label += " " + hint.Version
			}
			field("Static (low confidence)", label)
			field("Matched text", hint.Match)
		}
	}
	_, _ = fmt.Fprintf(w, "---\n\n%.1fms\n", a.DurationMS)
}
