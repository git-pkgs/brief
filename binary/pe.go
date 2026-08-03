package binary

import (
	"bytes"
	"debug/pe"
	stdbin "encoding/binary"
	"fmt"
	"io"
)

const (
	// IMAGE_IMPORT_DESCRIPTOR is 20 bytes; the DLL name RVA sits at
	// offset 12. The table is terminated by an all-zero descriptor.
	peImportDescriptorLen  = 20
	peImportDescriptorName = 12
	peDirectoryEntryImport = 1
	peMaxDLLNameLen        = 256
)

func inspectPE(r io.ReaderAt) (*Object, []byte, error) {
	f, err := pe.NewFile(r)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = f.Close() }()

	obj := &Object{
		Format: "pe",
		Arch:   peArch(f.Machine),
		Needed: peImportedLibraries(f),
	}

	return obj, peROData(f), nil
}

// peImportedLibraries reads DLL names from the import directory.
// pe.File.ImportedLibraries is an unimplemented stub in the standard library,
// and pe.File.ImportedSymbols omits libraries whose imports are all by
// ordinal, so read IMAGE_IMPORT_DESCRIPTOR entries directly.
func peImportedLibraries(f *pe.File) []string {
	dirs := peDataDirectories(f)
	if len(dirs) <= peDirectoryEntryImport {
		return nil
	}
	dir := dirs[peDirectoryEntryImport]
	if dir.VirtualAddress == 0 || dir.Size == 0 {
		return nil
	}

	sec := peSectionForRVA(f, dir.VirtualAddress)
	if sec == nil {
		return nil
	}
	data, err := sec.Data()
	if err != nil {
		return nil
	}
	off := int64(dir.VirtualAddress) - int64(sec.VirtualAddress)
	end := off + int64(dir.Size)
	if off < 0 || off >= int64(len(data)) {
		return nil
	}
	if end > int64(len(data)) {
		end = int64(len(data))
	}

	var libs []string
	seen := make(map[string]bool)
	for p := data[off:end]; len(p) >= peImportDescriptorLen; p = p[peImportDescriptorLen:] {
		desc := p[:peImportDescriptorLen]
		if isZero(desc) {
			break
		}
		nameRVA := stdbin.LittleEndian.Uint32(desc[peImportDescriptorName:])
		name := peStringAtRVA(f, nameRVA)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		libs = append(libs, name)
	}
	return libs
}

func peDataDirectories(f *pe.File) []pe.DataDirectory {
	switch h := f.OptionalHeader.(type) {
	case *pe.OptionalHeader32:
		return h.DataDirectory[:]
	case *pe.OptionalHeader64:
		return h.DataDirectory[:]
	default:
		return nil
	}
}

func peSectionForRVA(f *pe.File, rva uint32) *pe.Section {
	for _, s := range f.Sections {
		// Some linkers emit VirtualSize < SizeOfRawData; the Windows
		// loader treats the section extent as the larger of the two.
		end := uint64(s.VirtualAddress) + uint64(max(s.VirtualSize, s.Size))
		if uint64(rva) >= uint64(s.VirtualAddress) && uint64(rva) < end {
			return s
		}
	}
	return nil
}

func peStringAtRVA(f *pe.File, rva uint32) string {
	sec := peSectionForRVA(f, rva)
	if sec == nil {
		return ""
	}
	data, err := sec.Data()
	if err != nil {
		return ""
	}
	off := int64(rva) - int64(sec.VirtualAddress)
	if off < 0 || off >= int64(len(data)) {
		return ""
	}
	s := data[off:]
	if i := bytes.IndexByte(s, 0); i >= 0 {
		s = s[:i]
	}
	if len(s) > peMaxDLLNameLen {
		s = s[:peMaxDLLNameLen]
	}
	return string(s)
}

func isZero(b []byte) bool {
	for _, c := range b {
		if c != 0 {
			return false
		}
	}
	return true
}

func peROData(f *pe.File) []byte {
	var buf bytes.Buffer
	for _, name := range []string{".rdata", ".rodata"} {
		if sec := f.Section(name); sec != nil {
			if data, err := sec.Data(); err == nil {
				buf.Write(data)
			}
		}
	}
	return buf.Bytes()
}

func peArch(m uint16) string {
	switch m {
	case pe.IMAGE_FILE_MACHINE_AMD64:
		return archAMD64
	case pe.IMAGE_FILE_MACHINE_I386:
		return arch386
	case pe.IMAGE_FILE_MACHINE_ARM64:
		return archARM64
	case pe.IMAGE_FILE_MACHINE_ARMNT:
		return archARM
	case pe.IMAGE_FILE_MACHINE_RISCV64:
		return archRISCV64
	case pe.IMAGE_FILE_MACHINE_LOONGARCH64:
		return archLoong64
	default:
		return fmt.Sprintf("machine(0x%x)", m)
	}
}
