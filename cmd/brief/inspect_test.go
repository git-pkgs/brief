package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	stdbin "encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/git-pkgs/archives"
	"github.com/git-pkgs/magic"
	"github.com/ulikunitz/xz"

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

func TestInspectPathPEHeaderBeyondMagicPrefix(t *testing.T) {
	path := writeLargeStubPE(t)
	if !shouldAutoInspect(path) {
		t.Fatal("PE with a large DOS stub should auto-inspect")
	}

	art, err := inspectPath(path)
	if err != nil {
		t.Fatal(err)
	}
	if art.Format != magic.FormatPE {
		t.Fatalf("Format = %q, want %q", art.Format, magic.FormatPE)
	}
	if len(art.NativeObjects) != 1 || art.NativeObjects[0].Format != magic.FormatPE {
		t.Fatalf("NativeObjects = %+v, want one PE object", art.NativeObjects)
	}
}

func TestInspectPathArchivePEHeaderBeyondMagicPrefix(t *testing.T) {
	pe := writeLargeStubPE(t)
	archive := writeZip(t, map[string]string{"bin/large-stub.exe": pe})

	art, err := inspectPath(archive)
	if err != nil {
		t.Fatal(err)
	}
	if len(art.NativeObjects) != 1 || art.NativeObjects[0].Format != magic.FormatPE {
		t.Fatalf("NativeObjects = %+v, want one PE object", art.NativeObjects)
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

func TestInspectPathUsesDetectedArchiveFormat(t *testing.T) {
	archive := writeZip(t, map[string]string{
		"README.txt": "zip content\n",
	})
	mislabeled := filepath.Join(filepath.Dir(archive), "fixture.tar.gz")
	if err := os.Rename(archive, mislabeled); err != nil {
		t.Fatal(err)
	}

	art, err := inspectPath(mislabeled)
	if err != nil {
		t.Fatal(err)
	}
	if art.Format != "zip" {
		t.Fatalf("Format = %q, want zip", art.Format)
	}
	if art.Entries != 1 {
		t.Fatalf("Entries = %d, want 1", art.Entries)
	}
}

func TestInspectPathRejectsDuplicateArchivePaths(t *testing.T) {
	archive := writeDuplicateZip(t)
	if _, err := inspectPath(archive); !errors.Is(err, errArchiveDuplicatePath) {
		t.Fatalf("inspectPath error = %v, want errArchiveDuplicatePath", err)
	}
}

func TestInspectPathHandlesRestrictiveArchiveModes(t *testing.T) {
	t.Run("file", func(t *testing.T) {
		archive := writeModeTar(t, []tarEntry{
			{name: "locked", mode: 0, content: "plain text\n"},
		})
		art, err := inspectPath(archive)
		if err != nil {
			t.Fatal(err)
		}
		if art.Entries != 1 {
			t.Fatalf("Entries = %d, want 1", art.Entries)
		}
	})

	t.Run("directory", func(t *testing.T) {
		archive := writeModeTar(t, []tarEntry{
			{name: "locked/", mode: 0, directory: true},
			{name: "locked/file", mode: 0o600, content: "plain text\n"},
		})
		art, err := inspectPath(archive)
		if err != nil {
			t.Fatal(err)
		}
		if art.Entries != 1 {
			t.Fatalf("Entries = %d, want 1", art.Entries)
		}
	})
}

func TestInspectPathCaseDistinctArchiveEntries(t *testing.T) {
	archive := writeZip(t, map[string]string{"A": "one", "a": "two"})
	caseInsensitive, err := filesystemCaseInsensitive(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	art, err := inspectPath(archive)
	if caseInsensitive {
		if !errors.Is(err, errArchiveDuplicatePath) {
			t.Fatalf("inspectPath error = %v, want errArchiveDuplicatePath", err)
		}
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	if art.Entries != 2 {
		t.Fatalf("Entries = %d, want 2", art.Entries)
	}
}

func TestArchiveInputLimits(t *testing.T) {
	t.Run("input bytes", func(t *testing.T) {
		err := checkArchiveInputSize(maxArchiveInputBytes + 1)
		if !errors.Is(err, errArchiveLimit) {
			t.Fatalf("checkArchiveInputSize error = %v, want errArchiveLimit", err)
		}
	})

	t.Run("streamed input bytes", func(t *testing.T) {
		r := newArchiveInputLimitReader(strings.NewReader("1234"), 3)
		if _, err := io.ReadAll(r); !errors.Is(err, errArchiveLimit) {
			t.Fatalf("ReadAll error = %v, want errArchiveLimit", err)
		}
	})
}

func TestArchivePreflightLimits(t *testing.T) {
	t.Run("zip entry preflight", func(t *testing.T) {
		archive := writeZip(t, map[string]string{"a": "one", "b": "two"})
		data, err := os.ReadFile(archive)
		if err != nil {
			t.Fatal(err)
		}
		end := bytes.LastIndex(data, []byte{'P', 'K', 0x05, 0x06})
		if end < 0 {
			t.Fatal("zip end record not found")
		}
		// Advertise one record even though the central directory contains two.
		// The preflight must scan headers instead of trusting this count.
		stdbin.LittleEndian.PutUint16(data[end+8:end+10], 1)
		stdbin.LittleEndian.PutUint16(data[end+10:end+12], 1)
		if err := os.WriteFile(archive, data, 0o644); err != nil {
			t.Fatal(err)
		}
		f, err := os.Open(archive)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = f.Close() }()
		if err := preflightArtifactArchive(f, archive, magic.FormatZIP, 1, 10); !errors.Is(err, errArchiveLimit) {
			t.Fatalf("preflightArtifactArchive error = %v, want errArchiveLimit", err)
		}
	})

	t.Run("empty zip preflight", func(t *testing.T) {
		archive := writeZip(t, map[string]string{})
		f, err := os.Open(archive)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = f.Close() }()
		if err := preflightArtifactArchive(f, archive, magic.FormatZIP, 1, 10); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("tar entry preflight", func(t *testing.T) {
		archive := writeTar(t, map[string]string{"a": "one", "b": "two"})
		f, err := os.Open(archive)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = f.Close() }()
		if err := preflightArtifactArchive(f, archive, magic.FormatTAR, 1, 10); !errors.Is(err, errArchiveLimit) {
			t.Fatalf("preflightArtifactArchive error = %v, want errArchiveLimit", err)
		}
	})
}

func TestXZArchivePreflight(t *testing.T) {
	t.Run("valid dictionary", func(t *testing.T) {
		archive := writeTarXZ(t, map[string]string{"file": "plain text\n"})
		art, err := inspectPath(archive)
		if err != nil {
			t.Fatal(err)
		}
		if art.Entries != 1 {
			t.Fatalf("Entries = %d, want 1", art.Entries)
		}
	})

	t.Run("multiple blocks", func(t *testing.T) {
		archive := writeTarXZWithBlockSize(t, map[string]string{
			"file": strings.Repeat("plain text\n", 1_000),
		}, 512)
		art, err := inspectPath(archive)
		if err != nil {
			t.Fatal(err)
		}
		if art.Entries != 1 {
			t.Fatalf("Entries = %d, want 1", art.Entries)
		}
	})

	t.Run("oversized dictionary", func(t *testing.T) {
		archive := writeTarXZ(t, map[string]string{"file": "plain text\n"})
		setFirstXZDictionaryCode(t, archive, 40)
		f, err := os.Open(archive)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = f.Close() }()
		if err := preflightArtifactArchive(f, archive, magic.FormatXZ, 10, 10); !errors.Is(err, errArchiveLimit) {
			t.Fatalf("preflightArtifactArchive error = %v, want errArchiveLimit", err)
		}
	})
}

func TestArchiveReaderLimits(t *testing.T) {
	t.Run("entry count", func(t *testing.T) {
		r := &fakeArchiveReader{entries: []archives.FileInfo{
			{Path: "a", Size: 1},
			{Path: "b", Size: 1},
		}}
		if _, err := newArchiveLimitReader(r, 1, 10, false); !errors.Is(err, errArchiveLimit) {
			t.Fatalf("newArchiveLimitReader error = %v, want errArchiveLimit", err)
		}
	})

	t.Run("normalized duplicate paths", func(t *testing.T) {
		r := &fakeArchiveReader{entries: []archives.FileInfo{
			{Path: "entry", Size: 1},
			{Path: "./entry", Size: 1},
		}}
		if _, err := newArchiveLimitReader(r, 2, 10, false); !errors.Is(err, errArchiveDuplicatePath) {
			t.Fatalf("newArchiveLimitReader error = %v, want errArchiveDuplicatePath", err)
		}
	})

	t.Run("case-insensitive duplicate paths", func(t *testing.T) {
		r := &fakeArchiveReader{entries: []archives.FileInfo{
			{Path: "Entry", Size: 1},
			{Path: "entry", Size: 1},
		}}
		if _, err := newArchiveLimitReader(r, 2, 10, true); !errors.Is(err, errArchiveDuplicatePath) {
			t.Fatalf("newArchiveLimitReader error = %v, want errArchiveDuplicatePath", err)
		}
	})

	t.Run("declared bytes", func(t *testing.T) {
		r := &fakeArchiveReader{entries: []archives.FileInfo{{Path: "a", Size: 4}}}
		if _, err := newArchiveLimitReader(r, 1, 3, false); !errors.Is(err, errArchiveLimit) {
			t.Fatalf("newArchiveLimitReader error = %v, want errArchiveLimit", err)
		}
	})

	t.Run("actual bytes", func(t *testing.T) {
		r := &fakeArchiveReader{
			entries:  []archives.FileInfo{{Path: "a", Size: 1}},
			contents: map[string][]byte{"a": []byte("1234")},
		}
		limited, err := newArchiveLimitReader(r, 1, 3, false)
		if err != nil {
			t.Fatal(err)
		}
		if err := archives.ExtractAll(limited, t.TempDir()); !errors.Is(err, errArchiveLimit) {
			t.Fatalf("ExtractAll error = %v, want errArchiveLimit", err)
		}
	})
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

func TestEnableInspectGC(t *testing.T) {
	original := debug.SetGCPercent(-1)
	t.Cleanup(func() { debug.SetGCPercent(original) })

	enableInspectGC()
	got := debug.SetGCPercent(-1)
	debug.SetGCPercent(got)
	if got != inspectGCPercent {
		t.Fatalf("GC percent = %d, want %d", got, inspectGCPercent)
	}
}

func TestArtifactHumanSanitizesObjectFields(t *testing.T) {
	// A malicious archive could carry ANSI escapes in a soname or dylib
	// path, or line breaks in any field; the human formatter must strip them.
	esc := "\x1b[31m"
	injected := "\nInjected: yes\t"
	art := &brief.Artifact{
		Version: "test",
		Path:    "x" + injected,
		Format:  "elf",
		NativeObjects: []binary.Object{{
			Path:     "evil" + esc + injected,
			Format:   "elf",
			SOName:   "lib" + esc + injected + "red.so",
			Needed:   []string{"lib" + esc + injected + ".so"},
			Producer: []string{"GCC" + esc + injected},
			Go:       &binary.GoBuild{Version: "go1" + esc + injected, Main: "m" + esc + injected},
			Static:   []binary.Hint{{Library: "z" + esc + injected, Match: esc + injected}},
		}},
	}
	var buf bytes.Buffer
	report.ArtifactHuman(&buf, art)
	out := buf.String()
	if strings.Contains(out, "\x1b") {
		t.Fatalf("ANSI escape leaked into human output:\n%s", out)
	}
	if strings.Contains(out, "\nInjected:") || strings.Contains(out, "\t") {
		t.Fatalf("line-breaking whitespace leaked into human output:\n%s", out)
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

func writeLargeStubPE(tb testing.TB) string {
	tb.Helper()
	const headerOffset = 0x400
	data := make([]byte, headerOffset+24)
	copy(data[:2], "MZ")
	stdbin.LittleEndian.PutUint32(data[peHeaderOffsetAt:], headerOffset)
	copy(data[headerOffset:], []byte{'P', 'E', 0, 0})
	coff := data[headerOffset+peSignatureLen:]
	stdbin.LittleEndian.PutUint16(coff[0:2], 0x8664) // IMAGE_FILE_MACHINE_AMD64
	stdbin.LittleEndian.PutUint16(coff[18:20], 0x0002)

	path := filepath.Join(tb.TempDir(), "large-stub.exe")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		tb.Fatal(err)
	}
	return path
}

func writeDuplicateZip(tb testing.TB) string {
	tb.Helper()
	path := filepath.Join(tb.TempDir(), "duplicate.zip")
	f, err := os.Create(path)
	if err != nil {
		tb.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for _, content := range []string{"plain text\n", "second entry\n"} {
		w, err := zw.Create("entry")
		if err != nil {
			tb.Fatal(err)
		}
		if _, err := io.WriteString(w, content); err != nil {
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

func writeTar(tb testing.TB, entries map[string]string) string {
	tb.Helper()
	path := filepath.Join(tb.TempDir(), "fixture.tar")
	f, err := os.Create(path)
	if err != nil {
		tb.Fatal(err)
	}
	tw := tar.NewWriter(f)
	for name, content := range entries {
		header := &tar.Header{Name: name, Mode: 0o644, Size: int64(len(content))}
		if err := tw.WriteHeader(header); err != nil {
			tb.Fatal(err)
		}
		if _, err := io.WriteString(tw, content); err != nil {
			tb.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		tb.Fatal(err)
	}
	if err := f.Close(); err != nil {
		tb.Fatal(err)
	}
	return path
}

type tarEntry struct {
	name      string
	mode      int64
	content   string
	directory bool
}

func writeModeTar(tb testing.TB, entries []tarEntry) string {
	tb.Helper()
	path := filepath.Join(tb.TempDir(), "fixture.tar")
	f, err := os.Create(path)
	if err != nil {
		tb.Fatal(err)
	}
	tw := tar.NewWriter(f)
	for _, entry := range entries {
		typeflag := byte(tar.TypeReg)
		if entry.directory {
			typeflag = tar.TypeDir
		}
		header := &tar.Header{
			Name:     entry.name,
			Mode:     entry.mode,
			Size:     int64(len(entry.content)),
			Typeflag: typeflag,
		}
		if err := tw.WriteHeader(header); err != nil {
			tb.Fatal(err)
		}
		if _, err := io.WriteString(tw, entry.content); err != nil {
			tb.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		tb.Fatal(err)
	}
	if err := f.Close(); err != nil {
		tb.Fatal(err)
	}
	return path
}

func writeTarXZ(tb testing.TB, entries map[string]string) string {
	tb.Helper()
	return writeTarXZWithBlockSize(tb, entries, 0)
}

func writeTarXZWithBlockSize(tb testing.TB, entries map[string]string, blockSize int64) string {
	tb.Helper()
	path := filepath.Join(tb.TempDir(), "fixture.tar.xz")
	f, err := os.Create(path)
	if err != nil {
		tb.Fatal(err)
	}
	xw, err := xz.WriterConfig{DictCap: 1 << 20, BlockSize: blockSize}.NewWriter(f)
	if err != nil {
		tb.Fatal(err)
	}
	tw := tar.NewWriter(xw)
	for name, content := range entries {
		header := &tar.Header{Name: name, Mode: 0o644, Size: int64(len(content))}
		if err := tw.WriteHeader(header); err != nil {
			tb.Fatal(err)
		}
		if _, err := io.WriteString(tw, content); err != nil {
			tb.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		tb.Fatal(err)
	}
	if err := xw.Close(); err != nil {
		tb.Fatal(err)
	}
	if err := f.Close(); err != nil {
		tb.Fatal(err)
	}
	return path
}

func setFirstXZDictionaryCode(tb testing.TB, path string, code byte) {
	tb.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		tb.Fatal(err)
	}
	const xzStreamHeaderLen = 12
	if len(data) <= xzStreamHeaderLen {
		tb.Fatal("xz block header missing")
	}
	headerLen := (int(data[xzStreamHeaderLen]) + 1) * 4
	if headerLen < 12 || xzStreamHeaderLen+headerLen > len(data) {
		tb.Fatalf("invalid xz block header length %d", headerLen)
	}
	header := data[xzStreamHeaderLen : xzStreamHeaderLen+headerLen]
	if header[1] != 0 || header[2] != 0x21 || header[3] != 1 {
		tb.Fatalf("unexpected xz block header %x", header)
	}
	header[4] = code
	stdbin.LittleEndian.PutUint32(header[len(header)-4:], crc32.ChecksumIEEE(header[:len(header)-4]))
	if err := os.WriteFile(path, data, 0o644); err != nil {
		tb.Fatal(err)
	}
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

type fakeArchiveReader struct {
	entries  []archives.FileInfo
	contents map[string][]byte
}

func (r *fakeArchiveReader) List() ([]archives.FileInfo, error) {
	return r.entries, nil
}

func (r *fakeArchiveReader) ListDir(string) ([]archives.FileInfo, error) {
	return nil, nil
}

func (r *fakeArchiveReader) Extract(path string) (io.ReadCloser, error) {
	content, ok := r.contents[path]
	if !ok {
		return nil, fmt.Errorf("file not found: %s", path)
	}
	return io.NopCloser(bytes.NewReader(content)), nil
}

func (r *fakeArchiveReader) Hash(string) (string, error) {
	return "", nil
}

func (r *fakeArchiveReader) Close() error {
	return nil
}
