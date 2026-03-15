// Package report formats brief detection results for output.
package report

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode"

	"github.com/git-pkgs/brief"
)

// sanitize strips control characters (ANSI escapes, OSC sequences, etc.)
// from repo-controlled strings before printing to a terminal.
func sanitize(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\t' || r == '\n' {
			return r
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
}

// JSON writes the report as JSON.
func JSON(w io.Writer, r *brief.Report) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}

// Human writes the report in human-readable format.
func Human(w io.Writer, r *brief.Report, verbose bool) {
	_, _ = fmt.Fprintf(w, "brief %s — %s\n\n", r.Version, sanitize(r.Path))

	// Languages
	if len(r.Languages) > 0 {
		names := detectionNames(r.Languages)
		if len(names) == 1 {
			_, _ = fmt.Fprintf(w, "Language:        %s\n", names[0])
		} else {
			_, _ = fmt.Fprintf(w, "Language:        %s (also: %s)\n", names[0], strings.Join(names[1:], ", "))
		}
	}

	// Package managers
	for _, pm := range r.PackageManagers {
		line := pm.Name
		if pm.Command != nil {
			line += " (" + pm.Command.Run + ")"
		}
		_, _ = fmt.Fprintf(w, "Package Manager: %s\n", line)
		if pm.Lockfile != "" {
			_, _ = fmt.Fprintf(w, "                 Lockfile: %s\n", pm.Lockfile)
		}
	}

	// Dependencies (exclude CI actions from counts)
	if len(r.Dependencies) > 0 {
		directRuntime, directDev, totalRuntime, totalDev := 0, 0, 0, 0
		for _, d := range r.Dependencies {
			if strings.HasPrefix(d.PURL, "pkg:githubactions/") || strings.HasPrefix(d.PURL, "pkg:docker/") {
				continue
			}
			isDev := d.Scope == "development" || d.Scope == "test" || d.Scope == "build"
			if isDev {
				totalDev++
				if d.Direct {
					directDev++
				}
			} else {
				totalRuntime++
				if d.Direct {
					directRuntime++
				}
			}
		}
		var parts []string
		if directRuntime > 0 {
			s := fmt.Sprintf("%d runtime", directRuntime)
			if transitive := totalRuntime - directRuntime; transitive > 0 {
				s += fmt.Sprintf(" (%d total)", totalRuntime)
			}
			parts = append(parts, s)
		}
		if directDev > 0 {
			s := fmt.Sprintf("%d dev", directDev)
			if transitive := totalDev - directDev; transitive > 0 {
				s += fmt.Sprintf(" (%d total)", totalDev)
			}
			parts = append(parts, s)
		}
		if len(parts) > 0 {
			_, _ = fmt.Fprintf(w, "                 %s\n", strings.Join(parts, ", "))
		}
	}

	// Scripts
	if len(r.Scripts) > 0 {
		_, _ = fmt.Fprintln(w)
		source := r.Scripts[0].Source
		_, _ = fmt.Fprintf(w, "Scripts (%s):\n", source)
		for _, s := range r.Scripts {
			if s.Source != source {
				_, _ = fmt.Fprintf(w, "\nScripts (%s):\n", s.Source)
				source = s.Source
			}
			_, _ = fmt.Fprintf(w, "  %-8s %s\n", sanitize(s.Name)+":", sanitize(s.Run))
		}
	}

	_, _ = fmt.Fprintln(w)

	// Tool categories in a stable order
	categoryOrder := []string{"test", "lint", "format", "typecheck", "docs", "build", "codegen", "database", "security", "ci", "container", "infrastructure", "monorepo", "environment", "i18n", "release", "coverage", "dependency_bot"}
	categoryLabels := map[string]string{
		"test":           "Test",
		"lint":           "Lint",
		"format":         "Format",
		"typecheck":      "Typecheck",
		"docs":           "Docs",
		"build":          "Build",
		"codegen":        "Codegen",
		"database":       "Database",
		"security":       "Security",
		"ci":             "CI",
		"container":      "Container",
		"infrastructure": "Infra",
		"monorepo":       "Monorepo",
		"environment":    "Environment",
		"i18n":           "i18n",
		"release":        "Release",
		"coverage":       "Coverage",
		"dependency_bot": "Dep Updates",
	}

	for _, cat := range categoryOrder {
		label := categoryLabels[cat]
		if label == "" {
			label = cat
		}
		tools, ok := r.Tools[cat]
		if !ok {
			continue
		}
		for i, t := range tools {
			prefix := label + ":"
			if i > 0 {
				prefix = ""
			}
			line := t.Name
			if t.Command != nil {
				line += " (" + sanitize(t.Command.Run) + ")"
			}
			if len(t.ConfigFiles) > 0 {
				line += "  [" + sanitize(strings.Join(t.ConfigFiles, ", ")) + "]"
			}
			_, _ = fmt.Fprintf(w, "%-13s%s\n", prefix, line)

			if verbose {
				if t.Homepage != "" {
					_, _ = fmt.Fprintf(w, "              homepage: %s\n", t.Homepage)
				}
				if t.Docs != "" {
					_, _ = fmt.Fprintf(w, "              docs:     %s\n", t.Docs)
				}
			}
		}
	}

	// Print any categories not in the fixed order
	for cat, tools := range r.Tools {
		if categoryLabels[cat] != "" {
			continue
		}
		_, _ = fmt.Fprintln(w)
		for _, t := range tools {
			line := t.Name
			if t.Command != nil {
				line += " (" + t.Command.Run + ")"
			}
			_, _ = fmt.Fprintf(w, "%-13s%s\n", cat+":", line)
		}
	}

	// Style
	if r.Style != nil {
		_, _ = fmt.Fprintln(w)
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
			_, _ = fmt.Fprintf(w, "Style:       %s\n", strings.Join(parts, "  "))
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
			_, _ = fmt.Fprintf(w, "Layout:      %s\n", strings.Join(parts, " "))
		}
	}

	// Platforms
	if r.Platforms != nil {
		_, _ = fmt.Fprintln(w)
		for name, versions := range r.Platforms.CIMatrixVersions {
			_, _ = fmt.Fprintf(w, "Platforms:   %s %s (CI matrix)\n", name, strings.Join(versions, ", "))
		}
		for file, version := range r.Platforms.RuntimeVersionFiles {
			_, _ = fmt.Fprintf(w, "             %s: %s\n", sanitize(file), sanitize(version))
		}
		if len(r.Platforms.CIMatrixOS) > 0 {
			_, _ = fmt.Fprintf(w, "             OS: %s (CI matrix)\n", strings.Join(r.Platforms.CIMatrixOS, ", "))
		}
	}

	// Resources
	if r.Resources != nil {
		_, _ = fmt.Fprintln(w)
		res := r.Resources
		printResource(w, res.Readme)
		printResource(w, res.Contributing)
		printResource(w, res.Changelog)
		if res.License != "" {
			label := res.License
			if res.LicenseType != "" {
				label += " (" + res.LicenseType + ")"
			}
			_, _ = fmt.Fprintf(w, "Resources:   %s\n", label)
		}
		printResource(w, res.Security)
	}

	// Git
	if r.Git != nil {
		_, _ = fmt.Fprintln(w)
		if r.Git.Branch != "" {
			_, _ = fmt.Fprintf(w, "Git:         branch %s", sanitize(r.Git.Branch))
			if r.Git.DefaultBranch != "" && r.Git.DefaultBranch != r.Git.Branch {
				_, _ = fmt.Fprintf(w, " (default: %s)", sanitize(r.Git.DefaultBranch))
			}
			if r.Git.CommitCount > 0 {
				_, _ = fmt.Fprintf(w, "  %d commits", r.Git.CommitCount)
			}
			_, _ = fmt.Fprintln(w)
		}
		for name, url := range r.Git.Remotes {
			_, _ = fmt.Fprintf(w, "             %s: %s\n", sanitize(name), sanitize(url))
		}
	}

	// Lines
	if r.Lines != nil {
		_, _ = fmt.Fprintf(w, "\nLines:       %d code  %d files (%s)\n",
			r.Lines.TotalLines, r.Lines.TotalFiles, r.Lines.Source)
	}

	// Enrichment
	if r.Enrichment != nil {
		e := r.Enrichment
		if e.Repo != nil && e.Repo.Scorecard > 0 {
			_, _ = fmt.Fprintf(w, "\nScorecard:   %.1f/10 (%s)\n", e.Repo.Scorecard, e.Repo.ScorecardDate)
		}
		if len(e.RuntimeEOL) > 0 {
			_, _ = fmt.Fprintln(w)
			for name, eol := range e.RuntimeEOL {
				status := "supported"
				if !eol.Supported {
					status = "EOL"
				}
				if eol.LTS {
					status += ", LTS"
				}
				line := name + ": " + status
				if eol.Latest != "" {
					line += " (latest: " + eol.Latest + ")"
				}
				_, _ = fmt.Fprintf(w, "Runtime:     %s\n", line)
			}
		}
		if len(e.Packages) > 0 {
			_, _ = fmt.Fprintln(w)
			for _, pkg := range e.Packages {
				_, _ = fmt.Fprintf(w, "Published:   %s (%s)\n", pkg.Name, pkg.Ecosystem)
				if pkg.LatestVersion != "" {
					_, _ = fmt.Fprintf(w, "             latest: %s\n", pkg.LatestVersion)
				}
				if pkg.Downloads > 0 {
					_, _ = fmt.Fprintf(w, "             downloads: %d (%s)\n", pkg.Downloads, pkg.DownloadsPeriod)
				}
				if pkg.DependentReposCount > 0 {
					_, _ = fmt.Fprintf(w, "             dependents: %d repos, %d packages\n", pkg.DependentReposCount, pkg.DependentPackagesCount)
				}
				if pkg.RegistryURL != "" {
					_, _ = fmt.Fprintf(w, "             registry: %s\n", pkg.RegistryURL)
				}
			}
		}
	}

	// Stats
	_, _ = fmt.Fprintf(w, "\n%.1fms  %d files checked  %d/%d tools matched\n",
		r.Stats.DurationMS, r.Stats.FilesChecked, r.Stats.ToolsMatched, r.Stats.ToolsChecked)
}

func printResource(w io.Writer, value string) {
	if value != "" {
		_, _ = fmt.Fprintf(w, "Resources:   %s\n", value)
	}
}

func detectionNames(ds []brief.Detection) []string {
	names := make([]string, len(ds))
	for i, d := range ds {
		names[i] = d.Name
	}
	return names
}
