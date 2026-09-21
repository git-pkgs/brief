//go:build unix

package detect

import (
	"path/filepath"
	"slices"
	"syscall"
	"testing"
)

func TestProjectFileIndexExcludesNamedPipes(t *testing.T) {
	dir := t.TempDir()
	pipe := filepath.Join(dir, "blocked.toml")
	if err := syscall.Mkfifo(pipe, 0o600); err != nil {
		t.Fatal(err)
	}

	engine := New(loadKB(t), dir)
	engine.loadProjectFiles()

	if slices.Contains(engine.projectFiles, "blocked.toml") {
		t.Fatal("named pipe was included in the project file index")
	}
	if engine.exactFileExists("blocked.toml") {
		t.Fatal("named pipe was treated as a regular file")
	}
}

func TestCitationNamedPipe(t *testing.T) {
	t.Setenv("PATH", "")
	dir := t.TempDir()
	if err := syscall.Mkfifo(filepath.Join(dir, "CITATION.cff"), 0o600); err != nil {
		t.Fatal(err)
	}
	r, err := New(loadKB(t), dir).Run()
	if err != nil {
		t.Fatal(err)
	}
	if r.Resources != nil && r.Resources.Citation != nil && r.Resources.Citation.ParseStatus != "read_error" {
		t.Fatalf("named pipe treated as citation data: %+v", r.Resources.Citation)
	}
}
