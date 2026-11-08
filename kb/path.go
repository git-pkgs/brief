package kb

import (
	"path"
	"path/filepath"
	"strings"
)

// MatchPathPattern matches slash-separated paths and treats ** as zero or more
// complete path segments.
func MatchPathPattern(pattern, name string) bool {
	pattern = filepath.ToSlash(pattern)
	name = filepath.ToSlash(name)
	return matchPathSegments(strings.Split(pattern, "/"), strings.Split(name, "/"))
}

// MatchesPath reports whether the tool has positive file or config evidence at
// filePath. Exclusion patterns are not evidence for the tool.
func (tool *ToolDef) MatchesPath(filePath string) bool {
	if tool == nil {
		return false
	}
	patterns := make([]string, 0, len(tool.Detect.Files)+len(tool.Config.Files)+1)
	patterns = append(patterns, tool.Detect.Files...)
	patterns = append(patterns, tool.Config.Files...)
	if tool.Config.Lockfile != "" {
		patterns = append(patterns, tool.Config.Lockfile)
	}
	for target := range tool.Detect.FileContains {
		patterns = append(patterns, target)
	}
	for target := range tool.Detect.KeyExists {
		patterns = append(patterns, target)
	}
	return matchesAnyPath(patterns, filePath, false)
}

// MatchesPath reports whether filePath matches this resource definition.
func (resource ResourceInfo) MatchesPath(filePath string) bool {
	cleaned := strings.ToLower(filepath.ToSlash(filepath.Clean(filePath)))
	dir, name := path.Split(cleaned)
	dir = strings.TrimSuffix(dir, "/")
	if !matchesAnyPath(resource.Patterns, name, true) {
		return false
	}
	if resource.Recursive || dir == "" || dir == "." {
		return true
	}
	for _, allowed := range resource.Dirs {
		if strings.EqualFold(path.Clean(filepath.ToSlash(allowed)), dir) {
			return true
		}
	}
	return false
}

func matchesAnyPath(patterns []string, filePath string, foldCase bool) bool {
	if foldCase {
		filePath = strings.ToLower(filePath)
	}
	for _, pattern := range patterns {
		if foldCase {
			pattern = strings.ToLower(pattern)
		}
		if MatchPathPattern(pattern, filePath) {
			return true
		}
	}
	return false
}

func matchPathSegments(pattern, name []string) bool {
	if len(pattern) == 0 {
		return len(name) == 0
	}
	if pattern[0] == "**" {
		if matchPathSegments(pattern[1:], name) {
			return true
		}
		return len(name) > 0 && matchPathSegments(pattern, name[1:])
	}
	if len(name) == 0 {
		return false
	}
	matched, err := path.Match(pattern[0], name[0])
	return err == nil && matched && matchPathSegments(pattern[1:], name[1:])
}
