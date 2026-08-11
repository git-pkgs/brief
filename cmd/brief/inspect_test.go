package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/git-pkgs/brief"
	"github.com/git-pkgs/brief/binary"
	"github.com/git-pkgs/brief/report"
)

func TestInspectPathBareObject(t *testing.T) {
	bin := buildHello(t, runtime.GOOS, runtime.GOARCH)

	art, err := inspectPath(bin)
	if err != nil {
		t.Fatal(err)
	}
	if art.Format != hostObjectFormat() {
		t.Fatalf("Format = %q, want %q", art.Format, hostObjectFormat())
	}
	if art.Entries != 0 {
		t.Fatalf("Entries = %d, want 0 for bare object", art.Entries)
	}
	if art.SHA256 == "" {
		t.Fatal("SHA256 not set for bare object")
	}
	if len(art.NativeObjects) != 1 {
		t.Fatalf("NativeObjects = %d, want 1", len(art.NativeObjects))
	}
	obj := art.NativeObjects[0]
	if obj.Arch != runtime.GOARCH {
		t.Fatalf("Arch = %q, want %q", obj.Arch, runtime.GOARCH)
	}
	if obj.Go == nil || obj.Go.Path != "github.com/git-pkgs/brief/binary/testdata/hello" {
		t.Fatalf("Go = %+v, want testdata/hello module path", obj.Go)
	}

	// JSON round-trip.
	var buf bytes.Buffer
	if err := report.ArtifactJSON(&buf, art); err != nil {
		t.Fatal(err)
	}
	var back brief.Artifact
	if err := json.Unmarshal(buf.Bytes(), &back); err != nil {
		t.Fatalf("ArtifactJSON output does not decode: %v", err)
	}
	if back.NativeObjects[0].Arch != obj.Arch {
		t.Fatalf("round-trip Arch = %q, want %q", back.NativeObjects[0].Arch, obj.Arch)
	}
}

func TestInspectPathArchive(t *testing.T) {
	if testing.Short() {
		t.Skip("cross-compilation skipped in -short")
	}

	// Put objects for two different formats in one zip alongside a text
	// file so both the native-object filter and multi-entry sort are
	// exercised.
	elf := buildHello(t, "linux", "amd64")
	pe := buildHello(t, "windows", "amd64")
	archive := writeZip(t, map[string]string{
		"README.txt":      "not a binary\n",
		"lib/hello.so":    elf,
		"bin/hello.exe":   pe,
		"nested/deep.txt": "still not a binary\n",
	})

	art, err := inspectPath(archive)
	if err != nil {
		t.Fatal(err)
	}
	if art.Format != "zip" {
		t.Fatalf("Format = %q, want zip", art.Format)
	}
	if art.SHA256 == "" {
		t.Fatal("SHA256 not set for archive")
	}
	if art.Entries != 4 {
		t.Fatalf("Entries = %d, want 4", art.Entries)
	}
	if len(art.NativeObjects) != 2 {
		t.Fatalf("NativeObjects = %d, want 2", len(art.NativeObjects))
	}
	// Sorted by archive path.
	if art.NativeObjects[0].Path != "bin/hello.exe" || art.NativeObjects[0].Format != "pe" {
		t.Fatalf("NativeObjects[0] = %+v, want bin/hello.exe pe", art.NativeObjects[0])
	}
	if art.NativeObjects[1].Path != "lib/hello.so" || art.NativeObjects[1].Format != "elf" {
		t.Fatalf("NativeObjects[1] = %+v, want lib/hello.so elf", art.NativeObjects[1])
	}
}

func TestInspectPathNotAnArtifact(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plain.txt")
	if err := os.WriteFile(path, []byte("hello world\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := inspectPath(path); err == nil {
		t.Fatal("plain text file was accepted")
	}
}

func TestShouldAutoInspect(t *testing.T) {
	dir := t.TempDir()
	if shouldAutoInspect(dir) {
		t.Fatal("directory should not auto-inspect")
	}

	text := filepath.Join(dir, "text")
	if err := os.WriteFile(text, []byte("plain\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if shouldAutoInspect(text) {
		t.Fatal("text file should not auto-inspect")
	}

	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if !shouldAutoInspect(exe) {
		t.Fatal("test executable should auto-inspect")
	}

	if shouldAutoInspect(filepath.Join(dir, "missing")) {
		t.Fatal("missing file should not auto-inspect")
	}
}

func TestArtifactHumanSanitizesObjectFields(t *testing.T) {
	// A malicious archive could carry ANSI escapes in a soname or dylib
	// path; the human formatter must strip them.
	esc := "\x1b[31m"
	art := &brief.Artifact{
		Version: "test",
		Path:    "x",
		Format:  "elf",
		NativeObjects: []binary.Object{{
			Path:     "evil" + esc,
			Format:   "elf",
			SOName:   "lib" + esc + "red.so",
			Needed:   []string{"lib" + esc + ".so"},
			Producer: []string{"GCC" + esc},
			Go:       &binary.GoBuild{Version: "go1" + esc, Main: "m" + esc},
			Static:   []binary.Hint{{Library: "z" + esc, Match: esc}},
		}},
	}
	var buf bytes.Buffer
	report.ArtifactHuman(&buf, art)
	if strings.Contains(buf.String(), "\x1b") {
		t.Fatalf("ANSI escape leaked into human output:\n%s", buf.String())
	}
}

func BenchmarkInspectBareObject(b *testing.B) {
	bin := buildHello(b, runtime.GOOS, runtime.GOARCH)
	b.ReportAllocs()
	for b.Loop() {
		if _, err := inspectPath(bin); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkInspectArchive(b *testing.B) {
	elf := buildHello(b, "linux", "amd64")
	entries := map[string]string{"lib/hello.so": elf}
	// Pad with text files so the per-entry sniff cost is measured against
	// a realistic ratio of source to native objects.
	for i := range 50 {
		entries[fmt.Sprintf("src/pkg/file%02d.py", i)] = "# comment\n"
	}
	archive := writeZip(b, entries)

	b.ReportAllocs()
	for b.Loop() {
		if _, err := inspectPath(archive); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkShouldAutoInspect(b *testing.B) {
	exe, err := os.Executable()
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		shouldAutoInspect(exe)
	}
}

// buildHello cross-compiles binary/testdata/hello for goos/goarch and returns
// the output path. Skips the calling test if the target toolchain is
// unavailable.
func buildHello(tb testing.TB, goos, goarch string) string {
	tb.Helper()
	out := filepath.Join(tb.TempDir(), "hello")
	if goos == "windows" {
		out += ".exe"
	}
	cmd := exec.Command("go", "build", "-ldflags=-s -w", "-o", out, "github.com/git-pkgs/brief/binary/testdata/hello")
	cmd.Env = append(os.Environ(), "GOOS="+goos, "GOARCH="+goarch, "CGO_ENABLED=0")
	if b, err := cmd.CombinedOutput(); err != nil {
		tb.Skipf("cross-compile %s/%s: %v\n%s", goos, goarch, err, b)
	}
	return out
}

// writeZip creates a zip at a temp path. For each entry, if the value names an
// existing file its bytes are stored, otherwise the value itself is stored as
// literal content.
func writeZip(tb testing.TB, entries map[string]string) string {
	tb.Helper()
	path := filepath.Join(tb.TempDir(), "fixture.zip")
	f, err := os.Create(path)
	if err != nil {
		tb.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for name, val := range entries {
		w, err := zw.Create(name)
		if err != nil {
			tb.Fatal(err)
		}
		data := []byte(val)
		if b, err := os.ReadFile(val); err == nil {
			data = b
		}
		if _, err := w.Write(data); err != nil {
			tb.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		tb.Fatal(err)
	}
	if err := f.Close(); err != nil {
		tb.Fatal(err)
	}
	return path
}

func hostObjectFormat() string {
	switch runtime.GOOS {
	case "darwin", "ios":
		return "mach-o"
	case "windows":
		return "pe"
	default:
		return "elf"
	}
}
