// Package report formats brief detection results for output.
package report

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/git-pkgs/brief"
)

// JSON writes the report as JSON.
func JSON(w io.Writer, r *brief.Report) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}

// Human writes the report in human-readable format.
func Human(w io.Writer, r *brief.Report, verbose bool) {
	fmt.Fprintf(w, "brief %s — %s\n\n", r.Version, r.Path)

	// Languages
	if len(r.Languages) > 0 {
		names := detectionNames(r.Languages)
		if len(names) == 1 {
			fmt.Fprintf(w, "Language:        %s\n", names[0])
		} else {
			fmt.Fprintf(w, "Language:        %s (also: %s)\n", names[0], strings.Join(names[1:], ", "))
		}
	}

	// Package managers
	for _, pm := range r.PackageManagers {
		line := pm.Name
		if pm.Command != nil {
			line += " (" + pm.Command.Run + ")"
		}
		fmt.Fprintf(w, "Package Manager: %s\n", line)
	}

	// Scripts
	if len(r.Scripts) > 0 {
		fmt.Fprintln(w)
		source := r.Scripts[0].Source
		fmt.Fprintf(w, "Scripts (%s):\n", source)
		for _, s := range r.Scripts {
			if s.Source != source {
				fmt.Fprintf(w, "\nScripts (%s):\n", s.Source)
				source = s.Source
			}
			fmt.Fprintf(w, "  %-8s %s\n", s.Name+":", s.Run)
		}
	}

	fmt.Fprintln(w)

	// Tool categories in a stable order
	categoryOrder := []string{"test", "lint", "format", "typecheck", "docs", "build", "security", "ci", "container", "dependency_bot"}
	categoryLabels := map[string]string{
		"test":           "Test",
		"lint":           "Lint",
		"format":         "Format",
		"typecheck":      "Typecheck",
		"docs":           "Docs",
		"build":          "Build",
		"security":       "Security",
		"ci":             "CI",
		"container":      "Container",
		"dependency_bot": "Dep Updates",
	}

	for _, cat := range categoryOrder {
		label := categoryLabels[cat]
		if label == "" {
			label = cat
		}
		tools, ok := r.Tools[cat]
		if !ok {
			fmt.Fprintf(w, "%-13s—\n", label+":")
			continue
		}
		for i, t := range tools {
			prefix := label + ":"
			if i > 0 {
				prefix = ""
			}
			line := t.Name
			if t.Command != nil {
				line += " (" + t.Command.Run + ")"
			}
			if len(t.ConfigFiles) > 0 {
				line += "  [" + strings.Join(t.ConfigFiles, ", ") + "]"
			}
			fmt.Fprintf(w, "%-13s%s\n", prefix, line)

			if verbose {
				if t.Homepage != "" {
					fmt.Fprintf(w, "              homepage: %s\n", t.Homepage)
				}
				if t.Docs != "" {
					fmt.Fprintf(w, "              docs:     %s\n", t.Docs)
				}
			}
		}
	}

	// Print any categories not in the fixed order
	for cat, tools := range r.Tools {
		if categoryLabels[cat] != "" {
			continue
		}
		fmt.Fprintln(w)
		for _, t := range tools {
			line := t.Name
			if t.Command != nil {
				line += " (" + t.Command.Run + ")"
			}
			fmt.Fprintf(w, "%-13s%s\n", cat+":", line)
		}
	}

	// Style
	if r.Style != nil {
		fmt.Fprintln(w)
		parts := []string{}
		if r.Style.Indentation != "" {
			s := r.Style.Indentation
			if r.Style.IndentSource != "" {
				s += " (" + r.Style.IndentSource + ")"
			}
			parts = append(parts, s)
		}
		if r.Style.LineEnding != "" {
			parts = append(parts, r.Style.LineEnding)
		}
		if r.Style.TrailingNewline != nil {
			if *r.Style.TrailingNewline {
				parts = append(parts, "trailing newline")
			} else {
				parts = append(parts, "no trailing newline")
			}
		}
		if len(parts) > 0 {
			fmt.Fprintf(w, "Style:       %s\n", strings.Join(parts, "  "))
		}
	}

	// Layout
	if r.Layout != nil {
		parts := []string{}
		if len(r.Layout.SourceDirs) > 0 {
			parts = append(parts, strings.Join(r.Layout.SourceDirs, "/ ")+"/ ")
		}
		if len(r.Layout.TestDirs) > 0 {
			parts = append(parts, strings.Join(r.Layout.TestDirs, "/ ")+"/")
		}
		if len(parts) > 0 {
			fmt.Fprintf(w, "Layout:      %s\n", strings.Join(parts, " "))
		}
	}

	// Platforms
	if r.Platforms != nil {
		fmt.Fprintln(w)
		for file, version := range r.Platforms.RuntimeVersionFiles {
			fmt.Fprintf(w, "Runtime:     %s: %s\n", file, version)
		}
	}

	// Resources
	if r.Resources != nil {
		fmt.Fprintln(w)
		res := r.Resources
		printResource(w, "README", res.Readme)
		printResource(w, "CONTRIBUTING", res.Contributing)
		printResource(w, "CHANGELOG", res.Changelog)
		if res.License != "" {
			label := res.License
			if res.LicenseType != "" {
				label += " (" + res.LicenseType + ")"
			}
			fmt.Fprintf(w, "Resources:   %s\n", label)
		}
		printResource(w, "SECURITY", res.Security)
	}
}

func printResource(w io.Writer, label, value string) {
	if value != "" {
		fmt.Fprintf(w, "Resources:   %s\n", value)
	}
}

func detectionNames(ds []brief.Detection) []string {
	names := make([]string, len(ds))
	for i, d := range ds {
		names[i] = d.Name
	}
	return names
}
