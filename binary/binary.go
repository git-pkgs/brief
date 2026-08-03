// Package binary inspects native executable and shared-object files and
// reports their format, architecture, dynamic dependencies, and toolchain
// producer strings.
package binary

import (
	"debug/buildinfo"
	"errors"
	"fmt"
	"io"
	"os"
)

// Object describes a single native object file.
type Object struct {
	// Path is the filesystem path the object was read from.
	Path string `json:"path"`

	// Format is the container format: "elf", "mach-o", "mach-o-universal",
	// or "pe".
	Format string `json:"format"`

	// Arch is the target architecture in GOARCH form where a mapping
	// exists, otherwise the format's native name. For universal Mach-O
	// binaries it lists each slice separated by "/".
	Arch string `json:"arch,omitempty"`

	// SOName is the object's declared install name: DT_SONAME for ELF,
	// LC_ID_DYLIB for Mach-O.
	SOName string `json:"soname,omitempty"`

	// Needed lists dynamic dependencies: DT_NEEDED for ELF, LC_LOAD_DYLIB
	// and LC_LOAD_WEAK_DYLIB for Mach-O, and the PE import directory.
	Needed []string `json:"needed,omitempty"`

	// Producer lists toolchain identification strings recovered from the
	// object: the ELF .comment section (GCC, clang, and rustc all write
	// there), Mach-O LC_BUILD_VERSION, and the Go version when build info
	// is present.
	Producer []string `json:"producer,omitempty"`

	// Go is set when the object was produced by the Go toolchain and
	// carries embedded build metadata.
	Go *GoBuild `json:"go,omitempty"`

	// Static lists libraries that appear to be statically linked into the
	// object, inferred from version banners in read-only data. These are
	// heuristic matches and should be reported as low confidence.
	Static []Hint `json:"static,omitempty"`
}

// GoBuild is the subset of debug/buildinfo exposed in the report.
type GoBuild struct {
	Version string   `json:"version"`
	Path    string   `json:"path,omitempty"`
	Main    string   `json:"main,omitempty"`
	Deps    []string `json:"deps,omitempty"`
}

// Hint is a heuristic match for a statically-linked library.
type Hint struct {
	Library string `json:"library"`
	Version string `json:"version,omitempty"`
	Match   string `json:"match"`
}

// Arch values, in GOARCH form where a mapping exists.
const (
	archAMD64    = "amd64"
	arch386      = "386"
	archARM64    = "arm64"
	archARM64E   = "arm64e"
	archARM      = "arm"
	archRISCV32  = "riscv32"
	archRISCV64  = "riscv64"
	archPPC      = "ppc"
	archPPC64    = "ppc64"
	archPPC64LE  = "ppc64le"
	archS390     = "s390"
	archS390X    = "s390x"
	archMIPS     = "mips"
	archMIPSLE   = "mipsle"
	archMIPS64   = "mips64"
	archMIPS64LE = "mips64le"
	archLoong64  = "loong64"
)

// ErrUnrecognized is returned when the input is not a supported native
// object format.
var ErrUnrecognized = errors.New("unrecognized native object format")

// Inspect opens the file at path and returns its native object metadata.
func Inspect(path string) (*Object, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	info, err := f.Stat()
	if err != nil {
		return nil, err
	}

	obj, err := InspectReader(f, info.Size())
	if err != nil {
		return nil, err
	}
	obj.Path = path
	return obj, nil
}

// InspectReader reads a native object from r. All reads are confined to the
// first size bytes.
func InspectReader(r io.ReaderAt, size int64) (*Object, error) {
	const minHeader = 4
	if size < minHeader {
		return nil, ErrUnrecognized
	}
	sr := io.NewSectionReader(r, 0, size)

	var head [minHeader]byte
	if _, err := sr.ReadAt(head[:], 0); err != nil {
		return nil, fmt.Errorf("reading header: %w", err)
	}

	obj, rodata, err := dispatch(sr, size, head)
	if err != nil {
		return nil, err
	}

	// The fat Mach-O path reads build info from its first slice because
	// debug/buildinfo cannot open fat containers itself.
	if obj.Go == nil {
		if bi, err := buildinfo.Read(sr); err == nil {
			obj.Go = goBuildFrom(bi)
			obj.Producer = append(obj.Producer, bi.GoVersion)
		}
	}

	if len(rodata) > 0 {
		obj.Static = scanStatic(rodata)
	}

	return obj, nil
}

func dispatch(r *io.SectionReader, size int64, head [4]byte) (*Object, []byte, error) {
	switch {
	case head == [4]byte{0x7f, 'E', 'L', 'F'}:
		return inspectELF(r)
	case isMachO(head):
		return inspectMachO(r)
	case isMachOFat(head):
		return inspectMachOFat(r, size, head)
	case head[0] == 'M' && head[1] == 'Z':
		return inspectPE(r)
	default:
		return nil, nil, ErrUnrecognized
	}
}

func goBuildFrom(bi *buildinfo.BuildInfo) *GoBuild {
	g := &GoBuild{
		Version: bi.GoVersion,
		Path:    bi.Path,
	}
	if bi.Main.Path != "" {
		g.Main = bi.Main.Path + "@" + bi.Main.Version
	}
	for _, dep := range bi.Deps {
		if dep.Replace != nil {
			g.Deps = append(g.Deps, dep.Path+" => "+dep.Replace.Path+"@"+dep.Replace.Version)
		} else {
			g.Deps = append(g.Deps, dep.Path+"@"+dep.Version)
		}
	}
	return g
}
