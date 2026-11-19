package detect

import (
	"io/fs"
	"os"
	"path/filepath"

	"github.com/git-pkgs/gitignore"
)

const ignoreFileSizeLimit = 1 << 20

// loadIgnoreRules reads rules when the bounded scan reaches their directory.
// Each included submodule uses its own matcher, like a separate checkout.
func (e *Engine) loadIgnoreRules(dir string) {
	if e.TrackedOnly {
		return
	}
	file := filepath.Join(dir, ".gitignore")
	var info fs.FileInfo
	var err error
	if e.fsOnly {
		info, err = fs.Lstat(e.fsys, filepath.ToSlash(file))
	} else {
		info, err = os.Lstat(filepath.Join(e.Root, file))
	}
	if err != nil || !info.Mode().IsRegular() || info.Size() > ignoreFileSizeLimit {
		return
	}
	data, err := e.safeReadFileLimit(file, ignoreFileSizeLimit+1)
	if err != nil || len(data) == 0 || len(data) > ignoreFileSizeLimit {
		return
	}
	root := e.analysisRootFor(dir)
	local := e.pathAtAnalysisRoot(dir)
	if local == "." {
		local = ""
	}
	if e.ignoreMatchers == nil {
		e.ignoreMatchers = make(map[string]*gitignore.Matcher)
	}
	matcher := e.ignoreMatchers[root]
	if matcher == nil {
		matcher = gitignore.New("")
		e.ignoreMatchers[root] = matcher
	}
	matcher.AddPatterns(data, filepath.ToSlash(local))
}

func (e *Engine) ignoredProjectPath(rel string, isDir bool) bool {
	if e.TrackedOnly {
		return false
	}
	e.loadProjectFiles()
	rel = filepath.Clean(rel)
	matcher := e.ignoreMatchers[e.analysisRootFor(rel)]
	if matcher == nil || !matcher.MatchPath(filepath.ToSlash(e.pathAtAnalysisRoot(rel)), isDir) {
		return false
	}
	if e.fsOnly {
		return true
	}

	// A tracked file remains visible even if its name matches an ignore rule.
	// Only consult the index once a matching rule makes it necessary.
	if !e.ignoreTrackedLoaded {
		e.ignoreTrackedLoaded = true
		_ = e.loadTracked(e.Root)
	}
	if isDir {
		return !e.trackedDirs[rel] && !e.tracked[rel]
	}
	return !e.tracked[rel]
}

func (e *Engine) readEvidenceFile(file string) ([]byte, error) {
	if e.ignoredProjectPath(file, false) {
		return nil, fs.ErrNotExist
	}
	return e.safeReadFile(file)
}
