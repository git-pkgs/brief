package binary

import (
	"bytes"
	"debug/elf"
	stdbin "encoding/binary"
	"io"
)

func inspectELF(r io.ReaderAt) (*Object, []byte, error) {
	f, err := elf.NewFile(r)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = f.Close() }()

	obj := &Object{
		Format: "elf",
		Arch:   elfArch(f.Machine, f.Class, f.ByteOrder),
	}

	if needed, err := f.DynString(elf.DT_NEEDED); err == nil {
		obj.Needed = needed
	}
	if soname, err := f.DynString(elf.DT_SONAME); err == nil && len(soname) > 0 {
		obj.SOName = soname[0]
	}

	obj.Producer = elfComment(f)

	return obj, elfROData(f), nil
}

// elfComment returns the NUL-separated toolchain strings from the .comment
// section. GCC and clang both write entries of the form
// "GCC: (Debian 12.2.0-14) 12.2.0" or "clang version 17.0.6".
func elfComment(f *elf.File) []string {
	sec := f.Section(".comment")
	if sec == nil {
		return nil
	}
	data, err := sec.Data()
	if err != nil {
		return nil
	}
	var out []string
	seen := make(map[string]bool)
	for raw := range bytes.SplitSeq(data, []byte{0}) {
		s := string(bytes.TrimSpace(raw))
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

// elfROData returns the concatenation of sections likely to hold string
// literals for the static-link banner scan.
func elfROData(f *elf.File) []byte {
	var buf bytes.Buffer
	for _, name := range []string{".rodata", ".rdata", ".data.rel.ro"} {
		if sec := f.Section(name); sec != nil {
			if data, err := sec.Data(); err == nil {
				buf.Write(data)
			}
		}
	}
	return buf.Bytes()
}

func elfArch(m elf.Machine, class elf.Class, order stdbin.ByteOrder) string {
	is64 := class == elf.ELFCLASS64
	le := order == stdbin.LittleEndian
	switch m {
	case elf.EM_X86_64:
		return archAMD64
	case elf.EM_386:
		return arch386
	case elf.EM_AARCH64:
		return archARM64
	case elf.EM_ARM:
		return archARM
	case elf.EM_RISCV:
		if is64 {
			return archRISCV64
		}
		return archRISCV32
	case elf.EM_PPC64:
		if le {
			return archPPC64LE
		}
		return archPPC64
	case elf.EM_PPC:
		return archPPC
	case elf.EM_S390:
		if is64 {
			return archS390X
		}
		return archS390
	case elf.EM_MIPS:
		switch {
		case is64 && le:
			return archMIPS64LE
		case is64:
			return archMIPS64
		case le:
			return archMIPSLE
		default:
			return archMIPS
		}
	case elf.EM_LOONGARCH:
		return archLoong64
	default:
		return m.String()
	}
}
