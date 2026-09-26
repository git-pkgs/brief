package binary_test

import (
	"bytes"
	"debug/elf"
	"debug/macho"
	"encoding/binary"
	"testing"

	briefbinary "github.com/git-pkgs/brief/binary"
)

func TestInspectReaderWithoutGoMetadata(t *testing.T) {
	var elfData bytes.Buffer
	elfHeader := elf.Header64{
		Ident: [elf.EI_NIDENT]byte{0x7f, 'E', 'L', 'F', byte(elf.ELFCLASS64), byte(elf.ELFDATA2LSB), byte(elf.EV_CURRENT)},
		Type:  uint16(elf.ET_REL), Machine: uint16(elf.EM_X86_64), Version: uint32(elf.EV_CURRENT),
		Ehsize: 64,
	}
	if err := binary.Write(&elfData, binary.LittleEndian, elfHeader); err != nil {
		t.Fatal(err)
	}

	var machData bytes.Buffer
	machHeader := macho.FileHeader{Magic: macho.Magic64, Cpu: macho.CpuArm64, Type: macho.TypeObj}
	if err := binary.Write(&machData, binary.LittleEndian, machHeader); err != nil {
		t.Fatal(err)
	}
	if err := binary.Write(&machData, binary.LittleEndian, uint32(0)); err != nil {
		t.Fatal(err)
	}

	var fatData bytes.Buffer
	fatHeader := []uint32{macho.MagicFat, 1, uint32(macho.CpuArm64), 0, 28, uint32(machData.Len()), 0}
	if err := binary.Write(&fatData, binary.BigEndian, fatHeader); err != nil {
		t.Fatal(err)
	}
	fatData.Write(machData.Bytes())

	for _, tt := range []struct {
		format string
		arch   string
		data   []byte
	}{
		{"elf", "amd64", elfData.Bytes()},
		{"mach-o", "arm64", machData.Bytes()},
		{"mach-o-universal", "arm64", fatData.Bytes()},
	} {
		t.Run(tt.format, func(t *testing.T) {
			obj, err := briefbinary.InspectReader(bytes.NewReader(tt.data), int64(len(tt.data)))
			if err != nil {
				t.Fatal(err)
			}
			if obj.Format != tt.format || obj.Arch != tt.arch {
				t.Fatalf("object = %+v, want %s/%s", obj, tt.format, tt.arch)
			}
			if obj.Go != nil || len(obj.Producer) != 0 {
				t.Fatalf("unexpected Go metadata: Go=%+v, Producer=%v", obj.Go, obj.Producer)
			}
		})
	}
}
