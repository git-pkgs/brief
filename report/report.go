// Package report formats brief detection results for output.
package report

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
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

const maxDisplayItems = 20 // max items to show before truncating

// CategoryOrder defines the stable display order for tool categories.
var CategoryOrder = []string{"test", "lint", "format", "typecheck", "docs", "build", "codegen", "database", "security", "ci", "container", "infrastructure", "monorepo", "environment", "i18n", "release", "coverage", "dependency_bot"}

// CategoryLabels maps category keys to human-readable labels.
var CategoryLabels = map[string]string{
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

// Human writes the report in human-readable format.
func Human(w io.Writer, r *brief.Report, verbose bool) {
	_, _ = fmt.Fprintf(w, "brief %s — %s\n", r.Version, sanitize(r.Path))

	if r.DiffRef != "" {
		_, _ = fmt.Fprintf(w, "diff %s  (%d files changed)\n", r.DiffRef, len(r.ChangedFiles))
	}

	_, _ = fmt.Fprintln(w)

	printTruncatedList(w, "Commits:", r.DiffCommits)
	printTruncatedList(w, "Changed:", r.ChangedFiles)
	printLanguages(w, r.Languages)
	printPackageManagers(w, r.PackageManagers)
	printDependencySummary(w, r.Dependencies)
	printScripts(w, r.Scripts)

	_, _ = fmt.Fprintln(w)

	printTools(w, r.Tools, verbose)
	printStyle(w, r.Style)
	printLayout(w, r.Layout)
	printPlatforms(w, r.Platforms)
	printResources(w, r.Resources)
	printGit(w, r.Git)
	printLines(w, r.Lines)
	printEnrichment(w, r.Enrichment)

	_, _ = fmt.Fprintf(w, "\n%.1fms  %d files checked  %d/%d tools matched\n",
		r.Stats.DurationMS, r.Stats.FilesChecked, r.Stats.ToolsMatched, r.Stats.ToolsChecked)
}

func printTruncatedList(w io.Writer, header string, items []string) {
	if len(items) == 0 {
		return
	}
	_, _ = fmt.Fprintf(w, "%s\n", header)
	limit := min(len(items), maxDisplayItems)
	for _, item := range items[:limit] {
		_, _ = fmt.Fprintf(w, "  %s\n", sanitize(item))
	}
	if len(items) > maxDisplayItems {
		_, _ = fmt.Fprintf(w, "  ... and %d more\n", len(items)-maxDisplayItems)
	}
	_, _ = fmt.Fprintln(w)
}

func printLanguages(w io.Writer, languages []brief.Detection) {
	if len(languages) == 0 {
		return
	}
	names := detectionNames(languages)
	if len(names) == 1 {
		_, _ = fmt.Fprintf(w, "Language:        %s\n", names[0])
	} else {
		_, _ = fmt.Fprintf(w, "Language:        %s (also: %s)\n", names[0], strings.Join(names[1:], ", "))
	}
}

func printPackageManagers(w io.Writer, managers []brief.Detection) {
	for _, pm := range managers {
		line := pm.Name
		if pm.Command != nil {
			line += " (" + pm.Command.Run + ")"
		}
		_, _ = fmt.Fprintf(w, "Package Manager: %s\n", line)
		if pm.Lockfile != "" {
			_, _ = fmt.Fprintf(w, "                 Lockfile: %s\n", pm.Lockfile)
		}
	}
}

func printDependencySummary(w io.Writer, deps []brief.DepInfo) {
	if s := depSummary(deps); s != "" {
		_, _ = fmt.Fprintf(w, "                 %s\n", s)
	}
}

func printScripts(w io.Writer, scripts []brief.Script) {
	if len(scripts) == 0 {
		return
	}
	_, _ = fmt.Fprintln(w)
	source := scripts[0].Source
	_, _ = fmt.Fprintf(w, "Scripts (%s):\n", source)
	for _, s := range scripts {
		if s.Source != source {
			_, _ = fmt.Fprintf(w, "\nScripts (%s):\n", s.Source)
			source = s.Source
		}
		_, _ = fmt.Fprintf(w, "  %-8s %s\n", sanitize(s.Name)+":", sanitize(s.Run))
	}
}

func printTools(w io.Writer, tools map[string][]brief.Detection, verbose bool) {
	for _, cat := range CategoryOrder {
		label := CategoryLabels[cat]
		if label == "" {
			label = cat
		}
		dets, ok := tools[cat]
		if !ok {
			continue
		}
		printToolCategory(w, label, dets, verbose)
	}

	// Print any categories not in the fixed order
	for cat, dets := range tools {
		if CategoryLabels[cat] != "" {
			continue
		}
		_, _ = fmt.Fprintln(w)
		for _, t := range dets {
			line := t.Name
			if t.Command != nil {
				line += " (" + t.Command.Run + ")"
			}
			_, _ = fmt.Fprintf(w, "%-13s%s\n", cat+":", line)
		}
	}
}

func printToolCategory(w io.Writer, label string, tools []brief.Detection, verbose bool) {
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

func printStyle(w io.Writer, style *brief.StyleInfo) {
	if style == nil {
		return
	}
	_, _ = fmt.Fprintln(w)
	var parts []string
	if style.Indentation != "" {
		s := style.Indentation
		if style.IndentSource != "" {
			s += " (" + style.IndentSource + ")"
		}
		parts = append(parts, s)
	}
	if style.LineEnding != "" {
		parts = append(parts, style.LineEnding)
	}
	if style.TrailingNewline != nil {
		if *style.TrailingNewline {
			parts = append(parts, "trailing newline")
		} else {
			parts = append(parts, "no trailing newline")
		}
	}
	if len(parts) > 0 {
		_, _ = fmt.Fprintf(w, "Style:       %s\n", strings.Join(parts, "  "))
	}
}

func printLayout(w io.Writer, layout *brief.LayoutInfo) {
	if layout == nil {
		return
	}
	var parts []string
	if len(layout.SourceDirs) > 0 {
		parts = append(parts, "source: "+joinDirs(layout.SourceDirs))
	}
	if len(layout.TestDirs) > 0 {
		parts = append(parts, "test: "+joinDirs(layout.TestDirs))
	}
	if len(parts) > 0 {
		_, _ = fmt.Fprintf(w, "Layout:      %s\n", strings.Join(parts, "  "))
	}
}

// joinDirs formats directory names with trailing slashes.
func joinDirs(dirs []string) string {
	suffixed := make([]string, len(dirs))
	for i, d := range dirs {
		suffixed[i] = d + "/"
	}
	return strings.Join(suffixed, ", ")
}

func printPlatforms(w io.Writer, platforms *brief.PlatformInfo) {
	if platforms == nil {
		return
	}
	_, _ = fmt.Fprintln(w)
	for _, name := range sortedKeys(platforms.CIMatrixVersions) {
		_, _ = fmt.Fprintf(w, "Platforms:   %s %s (CI matrix)\n", name, strings.Join(platforms.CIMatrixVersions[name], ", "))
	}
	for _, file := range sortedKeys(platforms.RuntimeVersionFiles) {
		_, _ = fmt.Fprintf(w, "             %s: %s\n", sanitize(file), sanitize(platforms.RuntimeVersionFiles[file]))
	}
	if len(platforms.CIMatrixOS) > 0 {
		_, _ = fmt.Fprintf(w, "             OS: %s (CI matrix)\n", strings.Join(platforms.CIMatrixOS, ", "))
	}
}

func printResources(w io.Writer, res *brief.ResourceInfo) {
	if res == nil {
		return
	}
	_, _ = fmt.Fprintln(w)
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

func printGit(w io.Writer, git *brief.GitInfo) {
	if git == nil {
		return
	}
	_, _ = fmt.Fprintln(w)
	if git.Branch != "" {
		_, _ = fmt.Fprintf(w, "Git:         branch %s", sanitize(git.Branch))
		if git.DefaultBranch != "" && git.DefaultBranch != git.Branch {
			_, _ = fmt.Fprintf(w, " (default: %s)", sanitize(git.DefaultBranch))
		}
		if git.CommitCount > 0 {
			_, _ = fmt.Fprintf(w, "  %d commits", git.CommitCount)
		}
		_, _ = fmt.Fprintln(w)
	}
	for _, name := range sortedKeys(git.Remotes) {
		_, _ = fmt.Fprintf(w, "             %s: %s\n", sanitize(name), sanitize(git.Remotes[name]))
	}
}

func printLines(w io.Writer, lines *brief.LineCount) {
	if lines == nil {
		return
	}
	_, _ = fmt.Fprintf(w, "\nLines:       %d code  %d files (%s)\n",
		lines.TotalLines, lines.TotalFiles, lines.Source)
}

func printEnrichment(w io.Writer, e *brief.EnrichmentInfo) {
	if e == nil {
		return
	}
	if e.Repo != nil && e.Repo.Scorecard > 0 {
		_, _ = fmt.Fprintf(w, "\nScorecard:   %.1f/10 (%s)\n", e.Repo.Scorecard, e.Repo.ScorecardDate)
	}
	if len(e.RuntimeEOL) > 0 {
		_, _ = fmt.Fprintln(w)
		for name, eol := range e.RuntimeEOL {
			_, _ = fmt.Fprintf(w, "Runtime:     %s\n", enrichmentRuntimeLine(name, eol))
		}
	}
	for _, pkg := range e.Packages {
		_, _ = fmt.Fprintf(w, "Published:   %s (%s)\n", pkg.Name, pkg.Ecosystem)
		for _, line := range packageDetailLines(pkg) {
			_, _ = fmt.Fprintf(w, "             %s\n", line)
		}
	}
}

// MissingJSON writes the missing report as JSON.
func MissingJSON(w io.Writer, r *brief.MissingReport) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}

// ThreatJSON writes the threat report as JSON.
func ThreatJSON(w io.Writer, r *brief.ThreatReport) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}

// ThreatHuman writes the threat report in human-readable format.
func ThreatHuman(w io.Writer, r *brief.ThreatReport) {
	if len(r.Threats) == 0 {
		_, _ = fmt.Fprintln(w, "No security data available for detected tools.")
		return
	}

	if len(r.Ecosystems) > 0 {
		_, _ = fmt.Fprintf(w, "Detected: %s\n", strings.Join(r.Ecosystems, ", "))
	}
	if len(r.Stack) > 0 {
		names := make([]string, len(r.Stack))
		for i, s := range r.Stack {
			names[i] = s.Name
		}
		_, _ = fmt.Fprintf(w, "Stack:    %s\n", strings.Join(names, ", "))
	}
	_, _ = fmt.Fprintln(w)

	for _, t := range r.Threats {
		refs := t.CWE
		if t.OWASP != "" {
			if refs != "" {
				refs += " "
			}
			refs += t.OWASP
		}
		_, _ = fmt.Fprintf(w, "  %-18s %s  [%s]\n", t.ID, t.Title, refs)
		_, _ = fmt.Fprintf(w, "  %-18s via %s\n", "", strings.Join(t.IntroducedBy, ", "))
		if t.Note != "" {
			_, _ = fmt.Fprintf(w, "  %-18s %s\n", "", t.Note)
		}
		_, _ = fmt.Fprintln(w)
	}
}

// SinksJSON writes the sink report as JSON.
func SinksJSON(w io.Writer, r *brief.SinkReport) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}

// SinksHuman writes the sink report in human-readable format, grouped by tool.
func SinksHuman(w io.Writer, r *brief.SinkReport) {
	if len(r.Sinks) == 0 {
		_, _ = fmt.Fprintln(w, "No sink data available for detected tools.")
		return
	}

	currentTool := ""
	for _, s := range r.Sinks {
		if s.Tool != currentTool {
			if currentTool != "" {
				_, _ = fmt.Fprintln(w)
			}
			currentTool = s.Tool
			_, _ = fmt.Fprintf(w, "%s:\n", s.Tool)
		}
		line := fmt.Sprintf("  %-24s %-20s", s.Symbol, s.Threat)
		if s.CWE != "" {
			line += " " + s.CWE
		}
		_, _ = fmt.Fprintln(w, line)
		if s.Note != "" {
			_, _ = fmt.Fprintf(w, "  %-24s %s\n", "", s.Note)
		}
	}
}

// MissingHuman writes the missing report in human-readable format.
func MissingHuman(w io.Writer, r *brief.MissingReport) {
	if len(r.Missing) == 0 {
		_, _ = fmt.Fprintf(w, "No missing recommended tooling detected.\n")
		return
	}

	_, _ = fmt.Fprintf(w, "Detected: %s\n\n", strings.Join(r.Ecosystems, ", "))
	_, _ = fmt.Fprintf(w, "Missing recommended tooling:\n\n")

	for _, m := range r.Missing {
		_, _ = fmt.Fprintf(w, "  %-12s No %s tool configured\n", m.Label, strings.ToLower(m.Label))
		if m.Suggested != "" {
			line := "Suggested: " + m.Suggested
			if m.SuggestedCmd != "" {
				line += " (" + sanitize(m.SuggestedCmd) + ")"
			}
			_, _ = fmt.Fprintf(w, "  %-12s %s\n", "", line)
		}
		if m.Docs != "" {
			_, _ = fmt.Fprintf(w, "  %-12s %s\n", "", sanitize(m.Docs))
		}
		_, _ = fmt.Fprintln(w)
	}
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

// depSummary computes a human-readable dependency summary string.
// Returns empty string if there are no relevant dependencies.
func depSummary(deps []brief.DepInfo) string {
	if len(deps) == 0 {
		return ""
	}
	directRuntime, directDev, totalRuntime, totalDev := 0, 0, 0, 0
	for _, d := range deps {
		if strings.HasPrefix(d.PURL, "pkg:githubactions/") || strings.HasPrefix(d.PURL, "pkg:docker/") {
			continue
		}
		isDev := d.Scope == brief.ScopeDevelopment || d.Scope == brief.ScopeTest || d.Scope == brief.ScopeBuild
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
	return strings.Join(parts, ", ")
}

// enrichmentRuntimeLine formats a single runtime EOL entry.
func enrichmentRuntimeLine(name string, eol *brief.RuntimeEOL) string {
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
	return line
}

// packageDetailLines returns detail lines for a published package.
func packageDetailLines(pkg brief.PublishedPackage) []string {
	var lines []string
	if pkg.LatestVersion != "" {
		lines = append(lines, "latest: "+pkg.LatestVersion)
	}
	if pkg.Downloads > 0 {
		lines = append(lines, fmt.Sprintf("downloads: %d (%s)", pkg.Downloads, pkg.DownloadsPeriod))
	}
	if pkg.DependentReposCount > 0 {
		lines = append(lines, fmt.Sprintf("dependents: %d repos, %d packages", pkg.DependentReposCount, pkg.DependentPackagesCount))
	}
	if pkg.RegistryURL != "" {
		lines = append(lines, "registry: "+pkg.RegistryURL)
	}
	return lines
}

// sortedKeys returns the keys of a string-keyed map in sorted order.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
