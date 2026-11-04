package detect

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func (e *Engine) projectFS() fs.FS {
	if e.fsOnly {
		return e.fsys
	}
	return directoryFS{root: e.Root}
}

func (e *Engine) reportPath() (string, error) {
	if e.fsOnly {
		return ".", nil
	}
	return filepath.Abs(e.Root)
}

type directoryFS struct {
	root string
}

func (d directoryFS) Stat(name string) (fs.FileInfo, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrInvalid}
	}
	return os.Stat(filepath.Join(d.root, filepath.FromSlash(name)))
}

func (d directoryFS) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	file := filepath.Join(d.root, filepath.FromSlash(name))
	if name == "." {
		return os.Open(file)
	}
	info, err := os.Lstat(file)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return d.openSymlink(name, file)
	}
	if info.IsDir() {
		return os.Open(file)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("path is not a regular file: %s", name)
	}
	return openNoFollow(file)
}

func (d directoryFS) openSymlink(name, file string) (fs.File, error) {
	absPath, err := filepath.Abs(file)
	if err != nil {
		return nil, err
	}
	target, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		return nil, err
	}
	absRoot, err := filepath.Abs(d.root)
	if err != nil {
		return nil, err
	}
	resolvedRoot, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		return nil, err
	}
	rel, err := filepath.Rel(resolvedRoot, target)
	if err != nil {
		return nil, err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return nil, fmt.Errorf("symlink escapes project root: %s -> %s", name, target)
	}
	info, err := os.Stat(target)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("path is not a regular file: %s", name)
	}
	// Reject a symlink swapped into place after resolving the target.
	return openNoFollow(target)
}
