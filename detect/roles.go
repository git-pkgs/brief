package detect

import (
	"path/filepath"
	"strings"

	"github.com/git-pkgs/roles"
)

func (e *Engine) pathRoles(rel string) roles.Set {
	if labels, ok := e.fileRoles[rel]; ok {
		return labels
	}
	local := filepath.ToSlash(e.pathAtAnalysisRoot(rel))
	if strings.HasSuffix(rel, "/") {
		local += "/"
	}
	labels, err := roles.Match(local)
	if err != nil {
		return 0
	}
	return labels
}

func (e *Engine) projectEvidence(rel string) bool {
	labels := e.pathRoles(rel)
	return !labels.Has(roles.Vendor) && !labels.Has(roles.Fixture) &&
		!labels.Has(roles.Cache) && !labels.Has(roles.BuildOutput)
}

func (e *Engine) languageEvidence(rel string) bool {
	if !e.projectEvidence(rel) {
		return false
	}
	labels := e.pathRoles(rel)
	return !labels.Has(roles.Generated) && !labels.Has(roles.Example) &&
		!labels.Has(roles.Test) && !labels.Has(roles.Benchmark) &&
		!labels.Has(roles.Fuzz) && !labels.Has(roles.Documentation) &&
		!labels.Has(roles.Tooling)
}

func (e *Engine) sourceEvidence(rel string) bool {
	if !e.languageEvidence(rel) {
		return false
	}
	labels := e.pathRoles(rel)
	// Unclassified paths include root files and unconventional source directories.
	return (labels == 0 || labels.Has(roles.Source)) &&
		!labels.Has(roles.Packaging) && !labels.Has(roles.Configuration) &&
		!labels.Has(roles.Build) && !labels.Has(roles.CI) && !labels.Has(roles.Legal)
}

func (e *Engine) languageFileExists(pattern string) bool {
	e.filesChecked++
	e.loadProjectFiles()
	for _, rel := range e.projectFiles {
		if e.languageEvidence(rel) && e.matchesProjectPattern(pattern, rel) {
			return true
		}
	}
	return false
}

func (e *Engine) typeScriptDeclarationsOnly() *bool {
	e.loadProjectFiles()
	var hasDeclaration bool
	for _, rel := range e.projectFiles {
		ext := filepath.Ext(rel)
		switch ext {
		case ".ts", ".tsx", ".mts", ".cts":
			if !e.languageEvidence(rel) {
				continue
			}
			if ext == ".tsx" || !strings.HasSuffix(rel, ".d"+ext) {
				return new(false)
			}
			hasDeclaration = true
		}
	}
	if hasDeclaration {
		return new(true)
	}
	return nil
}
