package main

import (
	"bufio"
	"bytes"
	stdbin "encoding/binary"
	"errors"
	"fmt"
	"hash"
	"hash/crc32"
	"io"
)

const (
	// The largest dictionary used by the standard xz presets is 64 MiB.
	maxXZDictionaryBytes = 64 << 20

	xzStreamHeaderLen = 12
	xzStreamFooterLen = 12
	xzLZMA2FilterID   = 0x21
	xzMaxVLIBytes     = 10
	xzAlignment       = 4

	xzLZMA2CompressedControl          = 0x80
	xzLZMA2PropertiesControl          = 0xc0
	xzLZMA2UncompressedHighMask       = 0x1f
	xzLZMA2UncompressedHighShift      = 16
	xzLZMA2PropertiesCodeCount        = 9 * 5 * 5
	xzVLIContinuationBit              = 0x80
	xzVLIValueMask                    = 0x7f
	xzVLIGroupBits                    = 7
	xzMaxDictionaryCode               = 40
	xzDictionaryBase                  = 2
	xzDictionaryShiftBase             = 11
	xzMaximumDictionarySize           = 1<<32 - 1
	xzBlockHeaderChecksumSize         = 4
	xzIndexChecksumSize               = 4
	xzCheckCRC32                 byte = 1
	xzCheckCRC64                 byte = 4
	xzCheckSHA256                byte = 10
	xzCRC32Size                       = 4
	xzCRC64Size                       = 8
	xzSHA256Size                      = 32

	xzBlockCompressedSizePresent   = 0x40
	xzBlockUncompressedSizePresent = 0x80
	xzBlockReservedFlags           = 0x3c
)

var xzStreamMagic = []byte{0xfd, '7', 'z', 'X', 'Z', 0x00}

type xzBlockHeader struct {
	length           int
	compressedSize   int64
	uncompressedSize int64
}

type xzIndexRecord struct {
	unpaddedSize     int64
	uncompressedSize int64
}

type xzReader interface {
	io.Reader
	io.ByteReader
}

// preflightXZ validates every LZMA2 block header and chunk boundary before
// xz.NewReader can allocate the dictionary declared by an untrusted stream.
func preflightXZ(r io.Reader, maxDictionaryBytes uint64) error {
	br := bufio.NewReader(r)
	for {
		if err := preflightXZStream(br, maxDictionaryBytes); err != nil {
			return fmt.Errorf("reading xz: %w", err)
		}

		for {
			prefix, err := br.Peek(1)
			if errors.Is(err, io.EOF) {
				return nil
			}
			if err != nil {
				return fmt.Errorf("reading xz stream padding: %w", err)
			}
			if prefix[0] != 0 {
				break
			}
			var padding [4]byte
			if _, err := io.ReadFull(br, padding[:]); err != nil {
				return fmt.Errorf("reading xz stream padding: %w", err)
			}
			if !xzAllZeros(padding[:]) {
				return errors.New("xz stream padding is not aligned")
			}
		}
	}
}

func preflightXZStream(r xzReader, maxDictionaryBytes uint64) error {
	flags, checkSize, err := readXZStreamHeader(r)
	if err != nil {
		return err
	}

	var records []xzIndexRecord
	for {
		first, err := r.ReadByte()
		if err != nil {
			return err
		}
		if first == 0 {
			indexSize, err := readXZIndex(r, records)
			if err != nil {
				return err
			}
			return readXZStreamFooter(r, flags, indexSize)
		}

		header, err := readXZBlockHeader(r, first, maxDictionaryBytes)
		if err != nil {
			return err
		}
		compressedSize, uncompressedSize, err := scanXZLZMA2(r)
		if err != nil {
			return err
		}
		if header.compressedSize >= 0 && header.compressedSize != compressedSize {
			return errors.New("xz block compressed size does not match its header")
		}
		if header.uncompressedSize >= 0 && header.uncompressedSize != uncompressedSize {
			return errors.New("xz block uncompressed size does not match its header")
		}

		paddingLen := xzPadding(int64(header.length) + compressedSize)
		padding := make([]byte, paddingLen)
		if _, err := io.ReadFull(r, padding); err != nil {
			return err
		}
		if !xzAllZeros(padding) {
			return errors.New("xz block padding contains non-zero bytes")
		}
		if _, err := io.CopyN(io.Discard, r, int64(checkSize)); err != nil {
			return err
		}

		records = append(records, xzIndexRecord{
			unpaddedSize:     int64(header.length) + compressedSize + int64(checkSize),
			uncompressedSize: uncompressedSize,
		})
		if len(records) > maxArchiveEntries {
			return fmt.Errorf("%w: more than %d xz blocks", errArchiveLimit, maxArchiveEntries)
		}
	}
}

func readXZStreamHeader(r io.Reader) (flags byte, checkSize int, err error) {
	var header [xzStreamHeaderLen]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return 0, 0, err
	}
	if !bytes.Equal(header[:len(xzStreamMagic)], xzStreamMagic) {
		return 0, 0, errors.New("invalid xz stream header")
	}
	if header[6] != 0 || crc32.ChecksumIEEE(header[6:8]) != stdbin.LittleEndian.Uint32(header[8:]) {
		return 0, 0, errors.New("invalid xz stream flags")
	}
	checkSize, err = xzCheckSize(header[7])
	if err != nil {
		return 0, 0, err
	}
	return header[7], checkSize, nil
}

func readXZBlockHeader(
	r io.Reader,
	first byte,
	maxDictionaryBytes uint64,
) (xzBlockHeader, error) {
	headerLen := (int(first) + 1) * xzAlignment
	header := make([]byte, headerLen)
	header[0] = first
	if _, err := io.ReadFull(r, header[1:]); err != nil {
		return xzBlockHeader{}, err
	}
	contentEnd := headerLen - xzBlockHeaderChecksumSize
	if crc32.ChecksumIEEE(header[:contentEnd]) != stdbin.LittleEndian.Uint32(header[contentEnd:]) {
		return xzBlockHeader{}, errors.New("invalid xz block header checksum")
	}

	flags := header[1]
	if flags&xzBlockReservedFlags != 0 || flags&0x03 != 0 {
		return xzBlockHeader{}, errors.New("unsupported xz block flags")
	}
	br := bytes.NewReader(header[2:contentEnd])
	result := xzBlockHeader{length: headerLen, compressedSize: -1, uncompressedSize: -1}
	if flags&xzBlockCompressedSizePresent != 0 {
		size, _, err := readXZVLI(br)
		if err != nil || size > 1<<63-1 {
			return xzBlockHeader{}, errors.New("invalid xz block compressed size")
		}
		result.compressedSize = int64(size)
	}
	if flags&xzBlockUncompressedSizePresent != 0 {
		size, _, err := readXZVLI(br)
		if err != nil || size > 1<<63-1 {
			return xzBlockHeader{}, errors.New("invalid xz block uncompressed size")
		}
		result.uncompressedSize = int64(size)
	}

	filterID, _, err := readXZVLI(br)
	if err != nil || filterID != xzLZMA2FilterID {
		return xzBlockHeader{}, errors.New("unsupported xz block filter")
	}
	propertiesLen, _, err := readXZVLI(br)
	if err != nil || propertiesLen != 1 {
		return xzBlockHeader{}, errors.New("invalid xz LZMA2 filter properties")
	}
	dictionaryCode, err := br.ReadByte()
	if err != nil {
		return xzBlockHeader{}, err
	}
	dictionarySize, err := xzDictionarySize(dictionaryCode)
	if err != nil {
		return xzBlockHeader{}, err
	}
	if dictionarySize > maxDictionaryBytes {
		return xzBlockHeader{}, fmt.Errorf("%w: xz dictionary is %d bytes, limit is %d",
			errArchiveLimit, dictionarySize, maxDictionaryBytes)
	}
	if padding := header[contentEnd-br.Len() : contentEnd]; !xzAllZeros(padding) {
		return xzBlockHeader{}, errors.New("xz block header padding contains non-zero bytes")
	}
	return result, nil
}

func scanXZLZMA2(r xzReader) (compressedSize int64, uncompressedSize int64, err error) {
	for {
		control, err := r.ReadByte()
		if err != nil {
			return compressedSize, uncompressedSize, err
		}
		compressedSize++
		switch {
		case control == 0:
			return compressedSize, uncompressedSize, nil
		case control == 1 || control == 2:
			var header [2]byte
			if _, err := io.ReadFull(r, header[:]); err != nil {
				return compressedSize, uncompressedSize, err
			}
			size := int64(stdbin.BigEndian.Uint16(header[:])) + 1
			compressedSize += int64(len(header)) + size
			uncompressedSize += size
			if _, err := io.CopyN(io.Discard, r, size); err != nil {
				return compressedSize, uncompressedSize, err
			}
		case control >= xzLZMA2CompressedControl:
			var header [4]byte
			if _, err := io.ReadFull(r, header[:]); err != nil {
				return compressedSize, uncompressedSize, err
			}
			compressed := int64(stdbin.BigEndian.Uint16(header[2:])) + 1
			uncompressed := int64(control&xzLZMA2UncompressedHighMask)<<xzLZMA2UncompressedHighShift |
				int64(stdbin.BigEndian.Uint16(header[:2]))
			uncompressed++
			compressedSize += int64(len(header)) + compressed
			uncompressedSize += uncompressed
			if control >= xzLZMA2PropertiesControl {
				properties, err := r.ReadByte()
				if err != nil {
					return compressedSize, uncompressedSize, err
				}
				compressedSize++
				if properties >= xzLZMA2PropertiesCodeCount {
					return compressedSize, uncompressedSize, errors.New("invalid LZMA2 properties")
				}
			}
			if _, err := io.CopyN(io.Discard, r, compressed); err != nil {
				return compressedSize, uncompressedSize, err
			}
		default:
			return compressedSize, uncompressedSize, errors.New("invalid LZMA2 chunk control byte")
		}
	}
}

func readXZIndex(r xzReader, expected []xzIndexRecord) (int64, error) {
	crc := crc32.NewIEEE()
	_, _ = crc.Write([]byte{0})
	checked := &xzChecksumReader{reader: r, hash: crc}
	recordCount, n, err := readXZVLI(checked)
	consumed := int64(n + 1)
	if err != nil {
		return consumed, err
	}
	if recordCount != uint64(len(expected)) {
		return consumed, errors.New("xz index block count does not match the stream")
	}
	for _, want := range expected {
		unpadded, count, err := readXZVLI(checked)
		consumed += int64(count)
		if err != nil {
			return consumed, err
		}
		uncompressed, count, err := readXZVLI(checked)
		consumed += int64(count)
		if err != nil {
			return consumed, err
		}
		if unpadded != uint64(want.unpaddedSize) || uncompressed != uint64(want.uncompressedSize) {
			return consumed, errors.New("xz index record does not match its block")
		}
	}

	padding := make([]byte, xzPadding(consumed))
	if _, err := io.ReadFull(checked, padding); err != nil {
		return consumed, err
	}
	consumed += int64(len(padding))
	if !xzAllZeros(padding) {
		return consumed, errors.New("xz index padding contains non-zero bytes")
	}
	var checksum [xzIndexChecksumSize]byte
	if _, err := io.ReadFull(r, checksum[:]); err != nil {
		return consumed, err
	}
	if stdbin.LittleEndian.Uint32(checksum[:]) != crc.Sum32() {
		return consumed, errors.New("invalid xz index checksum")
	}
	return consumed + int64(len(checksum)), nil
}

func readXZStreamFooter(r io.Reader, flags byte, indexSize int64) error {
	var footer [xzStreamFooterLen]byte
	if _, err := io.ReadFull(r, footer[:]); err != nil {
		return err
	}
	if !bytes.Equal(footer[10:], []byte{'Y', 'Z'}) ||
		crc32.ChecksumIEEE(footer[4:10]) != stdbin.LittleEndian.Uint32(footer[:4]) {
		return errors.New("invalid xz stream footer")
	}
	if footer[8] != 0 || footer[9] != flags {
		return errors.New("xz stream footer flags do not match the header")
	}
	wantIndexSize := (int64(stdbin.LittleEndian.Uint32(footer[4:8])) + 1) * xzAlignment
	if indexSize != wantIndexSize {
		return errors.New("xz stream footer index size does not match")
	}
	return nil
}

type xzChecksumReader struct {
	reader io.Reader
	hash   hash.Hash32
}

func (r *xzChecksumReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	_, _ = r.hash.Write(p[:n])
	return n, err
}

func (r *xzChecksumReader) ReadByte() (byte, error) {
	var p [1]byte
	if _, err := io.ReadFull(r.reader, p[:]); err != nil {
		return 0, err
	}
	_, _ = r.hash.Write(p[:])
	return p[0], nil
}

func readXZVLI(r io.ByteReader) (uint64, int, error) {
	var value uint64
	for i, shift := 0, uint(0); ; i, shift = i+1, shift+xzVLIGroupBits {
		b, err := r.ReadByte()
		if err != nil {
			return value, i, err
		}
		if i >= xzMaxVLIBytes || i == xzMaxVLIBytes-1 && b > 1 {
			return value, i + 1, errors.New("xz VLI overflows uint64")
		}
		value |= uint64(b&xzVLIValueMask) << shift
		if b < xzVLIContinuationBit {
			return value, i + 1, nil
		}
	}
}

func xzDictionarySize(code byte) (uint64, error) {
	if code > xzMaxDictionaryCode {
		return 0, errors.New("invalid xz dictionary size code")
	}
	if code == xzMaxDictionaryCode {
		return xzMaximumDictionarySize, nil
	}
	return uint64(xzDictionaryBase|code&1) << (xzDictionaryShiftBase + (code >> 1)), nil
}

func xzCheckSize(flags byte) (int, error) {
	switch flags {
	case 0:
		return 0, nil
	case xzCheckCRC32:
		return xzCRC32Size, nil
	case xzCheckCRC64:
		return xzCRC64Size, nil
	case xzCheckSHA256:
		return xzSHA256Size, nil
	default:
		return 0, errors.New("unsupported xz integrity check")
	}
}

func xzPadding(size int64) int {
	if remainder := size % xzAlignment; remainder != 0 {
		return int(xzAlignment - remainder)
	}
	return 0
}

func xzAllZeros(data []byte) bool {
	for _, b := range data {
		if b != 0 {
			return false
		}
	}
	return true
}
