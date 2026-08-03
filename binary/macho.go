package binary

import (
	"bytes"
	"debug/buildinfo"
	"debug/macho"
	stdbin "encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Mach-O load commands not covered by debug/macho's typed Loads.
const (
	lcIDDylib       = 0xd
	lcLoadWeakDylib = 0x80000018
	lcBuildVersion  = 0x32

	loadCommandHeaderLen = 8 // cmd uint32 + cmdsize uint32
	cpuSubtypeMask       = 0x00ffffff
	cpuSubtypeARM64E     = 2

	// Packed version encoding used by minos/sdk fields: X.Y.Z stored as
	// XXXX.YY.ZZ in a uint32.
	versionMajorShift = 16
	versionMinorShift = 8
	versionByteMask   = 0xff
)

func isMachO(head [4]byte) bool {
	switch head {
	case [4]byte{0xcf, 0xfa, 0xed, 0xfe},
		[4]byte{0xce, 0xfa, 0xed, 0xfe},
		[4]byte{0xfe, 0xed, 0xfa, 0xcf},
		[4]byte{0xfe, 0xed, 0xfa, 0xce}:
		return true
	}
	return false
}

const (
	fatMagic32     = 0xcafebabe
	fatMagic64     = 0xcafebabf
	fatHeaderLen   = 8
	fatArch32Len   = 20
	fatArch64Len   = 32
	fatMaxArchSane = 64
)

func isMachOFat(head [4]byte) bool {
	m := stdbin.BigEndian.Uint32(head[:])
	return m == fatMagic32 || m == fatMagic64
}

func inspectMachO(r io.ReaderAt) (*Object, []byte, error) {
	f, err := macho.NewFile(r)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = f.Close() }()

	obj, rodata := machOFromFile(f, "mach-o")
	return obj, rodata, nil
}

// inspectMachOFat parses the fat header directly rather than delegating to
// debug/macho.NewFatFile, which only accepts FAT_MAGIC and rejects
// FAT_MAGIC_64.
func inspectMachOFat(r io.ReaderAt, size int64, head [4]byte) (*Object, []byte, error) {
	slices, err := readFatArches(r, size, head)
	if err != nil {
		return nil, nil, err
	}

	first, err := macho.NewFile(slices[0])
	if err != nil {
		return nil, nil, fmt.Errorf("mach-o fat slice 0: %w", err)
	}
	defer func() { _ = first.Close() }()

	// Report the first slice's dependencies and producer; list all
	// architectures in Arch.
	obj, rodata := machOFromFile(first, "mach-o-universal")
	arches := make([]string, 0, len(slices))
	arches = append(arches, obj.Arch)
	for i, sr := range slices[1:] {
		f, err := macho.NewFile(sr)
		if err != nil {
			return nil, nil, fmt.Errorf("mach-o fat slice %d: %w", i+1, err)
		}
		arches = append(arches, machOArch(f.Cpu, f.SubCpu))
		_ = f.Close()
	}
	obj.Arch = strings.Join(arches, "/")

	// debug/buildinfo cannot open fat containers of either width, so read
	// build metadata from the first slice directly.
	if bi, err := buildinfo.Read(slices[0]); err == nil {
		obj.Go = goBuildFrom(bi)
		obj.Producer = append(obj.Producer, bi.GoVersion)
	}

	return obj, rodata, nil
}

var errEmptyFat = errors.New("mach-o fat header has zero architectures")

func readFatArches(r io.ReaderAt, size int64, head [4]byte) ([]*io.SectionReader, error) {
	is64 := stdbin.BigEndian.Uint32(head[:]) == fatMagic64

	var hdr [fatHeaderLen]byte
	if _, err := r.ReadAt(hdr[:], 0); err != nil {
		return nil, err
	}
	narch := stdbin.BigEndian.Uint32(hdr[4:])
	if narch == 0 {
		return nil, errEmptyFat
	}
	if narch > fatMaxArchSane {
		return nil, fmt.Errorf("mach-o fat header claims %d architectures", narch)
	}

	entryLen := fatArch32Len
	if is64 {
		entryLen = fatArch64Len
	}
	table := make([]byte, int(narch)*entryLen)
	if _, err := r.ReadAt(table, fatHeaderLen); err != nil {
		return nil, err
	}
	tableEnd := int64(fatHeaderLen) + int64(len(table))

	out := make([]*io.SectionReader, 0, narch)
	for i := range int(narch) {
		entry := table[i*entryLen:]
		var off, sz int64
		if is64 {
			off = int64(stdbin.BigEndian.Uint64(entry[8:]))
			sz = int64(stdbin.BigEndian.Uint64(entry[16:]))
		} else {
			off = int64(stdbin.BigEndian.Uint32(entry[8:]))
			sz = int64(stdbin.BigEndian.Uint32(entry[12:]))
		}
		// off < tableEnd catches non-fat inputs that share the
		// 0xcafebabe magic (Java class files) as well as genuinely
		// corrupt headers whose slices overlap the arch table.
		if off < tableEnd || sz <= 0 || off+sz < off || off+sz > size {
			return nil, fmt.Errorf("mach-o fat arch %d out of bounds", i)
		}
		out = append(out, io.NewSectionReader(r, off, sz))
	}
	return out, nil
}

func machOFromFile(f *macho.File, format string) (*Object, []byte) {
	obj := &Object{
		Format: format,
		Arch:   machOArch(f.Cpu, f.SubCpu),
	}

	if libs, err := f.ImportedLibraries(); err == nil {
		obj.Needed = libs
	}

	for _, load := range f.Loads {
		raw := load.Raw()
		if len(raw) < loadCommandHeaderLen {
			continue
		}
		cmd := f.ByteOrder.Uint32(raw[0:4])
		switch cmd {
		case lcIDDylib:
			if name, ok := dylibName(raw, f.ByteOrder); ok {
				obj.SOName = name
			}
		case lcLoadWeakDylib:
			if name, ok := dylibName(raw, f.ByteOrder); ok {
				obj.Needed = append(obj.Needed, name)
			}
		case lcBuildVersion:
			if s := buildVersion(raw, f.ByteOrder); s != "" {
				obj.Producer = append(obj.Producer, s)
			}
		}
	}

	return obj, machOROData(f)
}

// dylibName decodes the path string from an LC_*_DYLIB command payload.
func dylibName(raw []byte, bo stdbin.ByteOrder) (string, bool) {
	// struct dylib_command { cmd; cmdsize; struct dylib { name.offset; ... } }
	const nameOffsetAt = 8
	if len(raw) < nameOffsetAt+4 {
		return "", false
	}
	off := bo.Uint32(raw[nameOffsetAt:])
	if int64(off) >= int64(len(raw)) {
		return "", false
	}
	name := raw[off:]
	if i := bytes.IndexByte(name, 0); i >= 0 {
		name = name[:i]
	}
	return string(name), len(name) > 0
}

// buildVersion decodes LC_BUILD_VERSION into a string like
// "macos 14.0 (sdk 14.2)".
func buildVersion(raw []byte, bo stdbin.ByteOrder) string {
	// struct build_version_command { cmd; cmdsize; platform; minos; sdk; ntools; }
	const headerLen = 24
	if len(raw) < headerLen {
		return ""
	}
	platform := bo.Uint32(raw[8:])
	minos := bo.Uint32(raw[12:])
	sdk := bo.Uint32(raw[16:])
	return fmt.Sprintf("%s %s (sdk %s)", machOPlatform(platform),
		machOVersion(minos), machOVersion(sdk))
}

func machOVersion(v uint32) string {
	return fmt.Sprintf("%d.%d.%d",
		v>>versionMajorShift,
		(v>>versionMinorShift)&versionByteMask,
		v&versionByteMask)
}

var machOPlatformNames = [...]string{
	1: "macos", 2: "ios", 3: "tvos", 4: "watchos", 5: "bridgeos",
	6: "maccatalyst", 7: "ios-simulator", 8: "tvos-simulator",
	9: "watchos-simulator", 10: "driverkit", 11: "visionos",
	12: "visionos-simulator",
}

func machOPlatform(p uint32) string {
	if p < uint32(len(machOPlatformNames)) && machOPlatformNames[p] != "" {
		return machOPlatformNames[p]
	}
	return fmt.Sprintf("platform(%d)", p)
}

func machOROData(f *macho.File) []byte {
	var buf bytes.Buffer
	for _, name := range []string{"__cstring", "__const", "__rodata"} {
		if sec := f.Section(name); sec != nil {
			if data, err := sec.Data(); err == nil {
				buf.Write(data)
			}
		}
	}
	return buf.Bytes()
}

func machOArch(cpu macho.Cpu, sub uint32) string {
	switch cpu {
	case macho.CpuAmd64:
		return archAMD64
	case macho.Cpu386:
		return arch386
	case macho.CpuArm64:
		if sub&cpuSubtypeMask == cpuSubtypeARM64E {
			return archARM64E
		}
		return archARM64
	case macho.CpuArm:
		return archARM
	case macho.CpuPpc:
		return archPPC
	case macho.CpuPpc64:
		return archPPC64
	default:
		return cpu.String()
	}
}
