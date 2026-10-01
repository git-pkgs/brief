package detect

import (
	"path"
	"path/filepath"
	"slices"

	"github.com/git-pkgs/brief/kb"
)

const (
	bundlerGemfile     = "Gemfile"
	bundlerGemfileLock = "Gemfile.lock"
	bundlerGemsRB      = "gems.rb"
	bundlerGemsLocked  = "gems.locked"
)

func (e *Engine) bundlerFiles(root string) (manifest, lockfile string) {
	root = filepath.ToSlash(root)
	if e.exactFileExists(path.Join(root, bundlerGemsRB)) {
		return path.Join(root, bundlerGemsRB), path.Join(root, bundlerGemsLocked)
	}
	return path.Join(root, bundlerGemfile), path.Join(root, bundlerGemfileLock)
}

func (e *Engine) activeBundlerFile(root, file string) bool {
	switch file {
	case bundlerGemfile, bundlerGemfileLock, bundlerGemsRB, bundlerGemsLocked:
		manifest, lockfile := e.bundlerFiles(root)
		candidate := path.Join(filepath.ToSlash(root), file)
		return candidate == manifest || candidate == lockfile
	default:
		return true
	}
}

func (e *Engine) detectLockfile(tool *kb.ToolDef) string {
	if slices.Contains(tool.Config.Files, bundlerGemsRB) {
		for _, root := range e.analysisRoots() {
			_, lockfile := e.bundlerFiles(root)
			if e.exactFileExists(lockfile) {
				return lockfile
			}
		}
		return ""
	}
	if tool.Config.Lockfile != "" {
		if found := e.findExisting([]string{tool.Config.Lockfile}); len(found) > 0 {
			return found[0]
		}
	}
	return ""
}
