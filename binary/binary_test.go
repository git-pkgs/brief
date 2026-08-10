package binary

import (
	"bytes"
	"debug/elf"
	stdbin "encoding/binary"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestInspectSelf(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	obj, err := Inspect(exe)
	if err != nil {
		t.Fatal(err)
	}

	wantFormat := hostFormat()
	if obj.Format != wantFormat {
		t.Fatalf("Format = %q, want %q", obj.Format, wantFormat)
	}
	if obj.Arch != runtime.GOARCH {
		t.Fatalf("Arch = %q, want %q", obj.Arch, runtime.GOARCH)
	}
	if obj.Go == nil {
		t.Fatal("Go build info not detected on Go test binary")
	}
	if !strings.HasPrefix(obj.Go.Version, "go") && !strings.HasPrefix(obj.Go.Version, "devel") {
		t.Fatalf("Go.Version = %q", obj.Go.Version)
	}
	if !slices.Contains(obj.Producer, obj.Go.Version) {
		t.Fatalf("Producer = %v, want to include %q", obj.Producer, obj.Go.Version)
	}
}

func TestInspectCrossFormats(t *testing.T) {
	if testing.Short() {
		t.Skip("cross-compilation skipped in -short")
	}

	tests := []struct {
		goos, goarch, format string
	}{
		{"linux", "amd64", "elf"},
		{"linux", "arm64", "elf"},
		{"linux", "ppc64le", "elf"},
		{"linux", "riscv64", "elf"},
		{"linux", "mips64le", "elf"},
		{"darwin", "arm64", "mach-o"},
		{"darwin", "amd64", "mach-o"},
		{"windows", "amd64", "pe"},
		{"windows", "arm64", "pe"},
	}

	for _, tt := range tests {
		t.Run(tt.goos+"/"+tt.goarch, func(t *testing.T) {
			t.Parallel()
			bin := buildFixture(t, tt.goos, tt.goarch)
			obj, err := Inspect(bin)
			if err != nil {
				t.Fatal(err)
			}
			if obj.Format != tt.format {
				t.Fatalf("Format = %q, want %q", obj.Format, tt.format)
			}
			if obj.Arch != tt.goarch {
				t.Fatalf("Arch = %q, want %q", obj.Arch, tt.goarch)
			}
			if obj.Go == nil || obj.Go.Path != "github.com/git-pkgs/brief/binary/testdata/hello" {
				t.Fatalf("Go = %+v, want testdata/hello module path", obj.Go)
			}
		})
	}
}

func TestInspectMachOUniversal(t *testing.T) {
	if testing.Short() {
		t.Skip("cross-compilation skipped in -short")
	}
	lipo, err := exec.LookPath("lipo")
	if err != nil {
		t.Skip("lipo not available")
	}

	amd64 := buildFixture(t, "darwin", "amd64")
	arm64 := buildFixture(t, "darwin", "arm64")

	for _, variant := range []struct{ name, flag string }{
		{"fat32", ""},
		{"fat64", "-fat64"},
	} {
		t.Run(variant.name, func(t *testing.T) {
			out := filepath.Join(t.TempDir(), "universal")
			args := []string{"-create"}
			if variant.flag != "" {
				args = append(args, variant.flag)
			}
			args = append(args, "-output", out, amd64, arm64)
			if b, err := exec.Command(lipo, args...).CombinedOutput(); err != nil {
				t.Fatalf("lipo: %v\n%s", err, b)
			}

			obj, err := Inspect(out)
			if err != nil {
				t.Fatal(err)
			}
			if obj.Format != "mach-o-universal" {
				t.Fatalf("Format = %q, want mach-o-universal", obj.Format)
			}
			if obj.Arch != "amd64/arm64" {
				t.Fatalf("Arch = %q, want amd64/arm64", obj.Arch)
			}
			if obj.Go == nil || obj.Go.Path != "github.com/git-pkgs/brief/binary/testdata/hello" {
				t.Fatalf("Go = %+v, want testdata/hello module path from first slice", obj.Go)
			}
		})
	}
}

func TestInspectMachOUniversalAggregatesSlices(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Mach-O linker tools are only available on darwin")
	}
	lipo, err := exec.LookPath("lipo")
	if err != nil {
		t.Skip("lipo not available")
	}
	clang, err := exec.LookPath("clang")
	if err != nil {
		t.Skip("clang not available")
	}

	dir := t.TempDir()
	plainSource := filepath.Join(dir, "plain.c")
	linkedSource := filepath.Join(dir, "linked.c")
	if err := os.WriteFile(plainSource, []byte("int main(void) { return 0; }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	linked := "#include <stdio.h>\n#include <zlib.h>\n" +
		"int main(void) { puts(zlibVersion()); puts(\"OpenSSL 3.0.13\"); return 0; }\n"
	if err := os.WriteFile(linkedSource, []byte(linked), 0o644); err != nil {
		t.Fatal(err)
	}

	amd64 := filepath.Join(dir, "plain-amd64")
	cmd := exec.Command(clang, "-arch", "x86_64", "-mmacosx-version-min=10.14", plainSource, "-o", amd64)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build amd64 fixture: %v\n%s", err, b)
	}
	arm64 := filepath.Join(dir, "linked-arm64")
	cmd = exec.Command(clang, "-arch", "arm64", "-mmacosx-version-min=11.0", linkedSource, "-lz", "-o", arm64)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build arm64 fixture: %v\n%s", err, b)
	}

	out := filepath.Join(dir, "universal")
	if b, err := exec.Command(lipo, "-create", "-output", out, amd64, arm64).CombinedOutput(); err != nil {
		t.Fatalf("lipo: %v\n%s", err, b)
	}
	amd64Obj, err := Inspect(amd64)
	if err != nil {
		t.Fatal(err)
	}
	arm64Obj, err := Inspect(arm64)
	if err != nil {
		t.Fatal(err)
	}
	obj, err := Inspect(out)
	if err != nil {
		t.Fatal(err)
	}

	var arm64OnlyDependency string
	for _, needed := range arm64Obj.Needed {
		if !slices.Contains(amd64Obj.Needed, needed) {
			arm64OnlyDependency = needed
			break
		}
	}
	if arm64OnlyDependency == "" {
		t.Fatal("arm64 fixture has no slice-specific dependency")
	}
	if !slices.Contains(obj.Needed, arm64OnlyDependency) {
		t.Fatalf("Needed = %v, want dependency %q from arm64 slice", obj.Needed, arm64OnlyDependency)
	}
	var arm64OnlyProducer string
	for _, producer := range arm64Obj.Producer {
		if !slices.Contains(amd64Obj.Producer, producer) {
			arm64OnlyProducer = producer
			break
		}
	}
	if arm64OnlyProducer == "" {
		t.Fatal("arm64 fixture has no slice-specific producer")
	}
	if !slices.Contains(obj.Producer, arm64OnlyProducer) {
		t.Fatalf("Producer = %v, want producer %q from arm64 slice", obj.Producer, arm64OnlyProducer)
	}
	if !slices.ContainsFunc(arm64Obj.Static, func(h Hint) bool { return h.Library == "openssl" }) {
		t.Fatal("arm64 fixture has no openssl static hint")
	}
	if !slices.ContainsFunc(obj.Static, func(h Hint) bool { return h.Library == "openssl" }) {
		t.Fatalf("Static = %v, want openssl hint from arm64 slice", obj.Static)
	}

	goArm64 := buildFixture(t, "darwin", "arm64")
	goOut := filepath.Join(dir, "universal-go-second")
	if b, err := exec.Command(lipo, "-create", "-output", goOut, amd64, goArm64).CombinedOutput(); err != nil {
		t.Fatalf("lipo Go fixture: %v\n%s", err, b)
	}
	goObj, err := Inspect(goOut)
	if err != nil {
		t.Fatal(err)
	}
	if goObj.Go == nil || goObj.Go.Path != "github.com/git-pkgs/brief/binary/testdata/hello" {
		t.Fatalf("Go = %+v, want build info from arm64 slice", goObj.Go)
	}
}

func TestInspectMachOFatRejectsMalformed(t *testing.T) {
	// Zero-arch fat header.
	empty := []byte("\xca\xfe\xba\xbe\x00\x00\x00\x00")
	if _, err := InspectReader(bytes.NewReader(empty), int64(len(empty))); !errors.Is(err, errEmptyFat) {
		t.Fatalf("zero-arch fat: err = %v, want errEmptyFat", err)
	}

	// One arch pointing at garbage inside the file.
	buf := make([]byte, 128)
	copy(buf, "\xca\xfe\xba\xbe\x00\x00\x00\x01")
	stdbin.BigEndian.PutUint32(buf[16:], 32) // offset
	stdbin.BigEndian.PutUint32(buf[20:], 64) // size
	if _, err := InspectReader(bytes.NewReader(buf), int64(len(buf))); err == nil {
		t.Fatal("fat header pointing at non-Mach-O slice was accepted")
	}

	// Slice extends past declared size.
	stdbin.BigEndian.PutUint32(buf[20:], 4096)
	if _, err := InspectReader(bytes.NewReader(buf), int64(len(buf))); err == nil {
		t.Fatal("out-of-bounds fat slice was accepted")
	}

	// Slice offset lands inside the arch table itself. Java class files
	// share the 0xcafebabe magic and typically hit this path.
	stdbin.BigEndian.PutUint32(buf[16:], 8)  // offset < tableEnd (28)
	stdbin.BigEndian.PutUint32(buf[20:], 64) // size
	if _, err := InspectReader(bytes.NewReader(buf), int64(len(buf))); err == nil {
		t.Fatal("fat slice overlapping the arch table was accepted")
	}
}

func TestDylibNameRejectsOverflowOffset(t *testing.T) {
	raw := make([]byte, 24)
	stdbin.LittleEndian.PutUint32(raw[8:], 0xffffffff)
	if name, ok := dylibName(raw, stdbin.LittleEndian); ok {
		t.Fatalf("dylibName accepted out-of-range offset: %q", name)
	}
}

func TestELFArch(t *testing.T) {
	tests := []struct {
		m     elf.Machine
		class elf.Class
		order stdbin.ByteOrder
		want  string
	}{
		{elf.EM_RISCV, elf.ELFCLASS64, stdbin.LittleEndian, "riscv64"},
		{elf.EM_RISCV, elf.ELFCLASS32, stdbin.LittleEndian, "riscv32"},
		{elf.EM_PPC64, elf.ELFCLASS64, stdbin.LittleEndian, "ppc64le"},
		{elf.EM_PPC64, elf.ELFCLASS64, stdbin.BigEndian, "ppc64"},
		{elf.EM_PPC, elf.ELFCLASS32, stdbin.BigEndian, "ppc"},
		{elf.EM_S390, elf.ELFCLASS64, stdbin.BigEndian, "s390x"},
		{elf.EM_S390, elf.ELFCLASS32, stdbin.BigEndian, "s390"},
		{elf.EM_MIPS, elf.ELFCLASS32, stdbin.BigEndian, "mips"},
		{elf.EM_MIPS, elf.ELFCLASS32, stdbin.LittleEndian, "mipsle"},
		{elf.EM_MIPS, elf.ELFCLASS64, stdbin.BigEndian, "mips64"},
		{elf.EM_MIPS, elf.ELFCLASS64, stdbin.LittleEndian, "mips64le"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := elfArch(tt.m, tt.class, tt.order); got != tt.want {
				t.Fatalf("elfArch(%v, %v, %v) = %q, want %q", tt.m, tt.class, tt.order, got, tt.want)
			}
		})
	}
}

func TestInspectReaderBoundsSize(t *testing.T) {
	// Use a real, valid executable so an unbounded implementation would
	// succeed: with the section-reader bound in place a size of 4 must
	// fail even though the underlying ReaderAt has the full file.
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	buf, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	r := bytes.NewReader(buf)

	if _, err := InspectReader(r, int64(len(buf))); err != nil {
		t.Fatalf("full-size control failed: %v", err)
	}
	if _, err := InspectReader(r, 4); err == nil {
		t.Fatal("InspectReader read past declared size")
	}
	if _, err := InspectReader(r, 0); !errors.Is(err, ErrUnrecognized) {
		t.Fatalf("size=0 error = %v, want ErrUnrecognized", err)
	}
	if _, err := InspectReader(r, -1); !errors.Is(err, ErrUnrecognized) {
		t.Fatalf("size<0 error = %v, want ErrUnrecognized", err)
	}
}

func TestInspectUnrecognized(t *testing.T) {
	path := filepath.Join(t.TempDir(), "text")
	if err := os.WriteFile(path, []byte("not a binary"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Inspect(path)
	if !errors.Is(err, ErrUnrecognized) {
		t.Fatalf("Inspect(text) error = %v, want ErrUnrecognized", err)
	}
}

func TestInspectShortFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "short")
	if err := os.WriteFile(path, []byte{0x7f}, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Inspect(path); err == nil {
		t.Fatal("Inspect on 1-byte file succeeded")
	}
}

func TestScanStatic(t *testing.T) {
	tests := []struct {
		name    string
		rodata  string
		library string
		version string
	}{
		{"zlib deflate", "xx\x00deflate 1.3.1 Copyright 1995-2024 Jean-loup Gailly\x00", "zlib", "1.3.1"},
		{"zlib inflate", "inflate 1.2.13 Copyright 1995-2022 Mark Adler", "zlib", "1.2.13"},
		{"openssl", "\x00OpenSSL 3.0.13 30 Jan 2024\x00", "openssl", "3.0.13"},
		{"openssl 1.1", "OpenSSL 1.1.1w  11 Sep 2023", "openssl", "1.1.1w"},
		{"sqlite", "\x002026-07-24 19:02:57 bf7c7f30031888f4e796e429ab3978879485813aaca6f641c7b33e4e09459bcc\x00", "sqlite", ""},
		{"libcurl", "libcurl/8.6.0", "libcurl", "8.6.0"},
		{"pcre2", "PCRE2 10.42 2022-12-11", "pcre2", "10.42"},
		{"libpng", "libpng version 1.6.43", "libpng", "1.6.43"},
		{"boringssl", "BoringSSL", "boringssl", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hints := scanStatic([]byte(tt.rodata))
			if len(hints) != 1 {
				t.Fatalf("scanStatic(%q) = %v, want one hit", tt.rodata, hints)
			}
			if hints[0].Library != tt.library || hints[0].Version != tt.version {
				t.Fatalf("hit = %+v, want %s@%s", hints[0], tt.library, tt.version)
			}
		})
	}
}

func TestScanStaticDedupes(t *testing.T) {
	// Two distinct patterns for the same library both match; the
	// version-capturing one is listed first in staticPatterns and must win.
	rodata := []byte("libxml2 2.12.6\x00xmlParseDoc : out of memory\x00")
	hints := scanStatic(rodata)
	if len(hints) != 1 {
		t.Fatalf("got %d hits, want 1 deduped", len(hints))
	}
	if hints[0].Library != "libxml2" || hints[0].Version != "2.12.6" {
		t.Fatalf("hit = %+v, want libxml2@2.12.6", hints[0])
	}
}

func TestScanStaticNoMatch(t *testing.T) {
	if hints := scanStatic([]byte("hello world 3.14.159")); hints != nil {
		t.Fatalf("got %v, want nil", hints)
	}
}

func buildFixture(t *testing.T, goos, goarch string) string {
	t.Helper()

	out := filepath.Join(t.TempDir(), "hello")
	if goos == "windows" {
		out += ".exe"
	}

	cmd := exec.Command("go", "build", "-ldflags=-s -w", "-o", out, "./testdata/hello")
	cmd.Env = append(os.Environ(), "GOOS="+goos, "GOARCH="+goarch, "CGO_ENABLED=0")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("cross-compile %s/%s: %v\n%s", goos, goarch, err, b)
	}
	return out
}

func hostFormat() string {
	switch runtime.GOOS {
	case "darwin", "ios":
		return "mach-o"
	case "windows":
		return "pe"
	default:
		return "elf"
	}
}
