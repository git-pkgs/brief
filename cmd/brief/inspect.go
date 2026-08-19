package main

import (
	"archive/tar"
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"crypto/sha256"
	stdbin "encoding/binary"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	pathpkg "path"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"
	"time"

	"github.com/git-pkgs/archives"
	"github.com/git-pkgs/magic"
	"github.com/ulikunitz/xz"

	"github.com/git-pkgs/brief"
	"github.com/git-pkgs/brief/binary"
	"github.com/git-pkgs/brief/report"
)

const (
	// magicSniffLen is enough for every native-object and archive prefix that
	// git-pkgs/magic recognises, including the PE header-offset indirection at
	// 0x3c and the tar checksum at offset 148.
	magicSniffLen = 512

	inspectGCPercent         = 100
	maxArchiveEntries        = 100_000
	maxArchiveInputBytes     = 512 << 20
	maxArchiveExtractedBytes = 512 << 20
	archiveDirectoryAccess   = 0o700

	peHeaderOffsetAt = 0x3c
	peSignatureLen   = 4

	zipDirectoryHeaderSignature = 0x02014b50
	zipDirectoryEndSignature    = 0x06054b50
	zipDirectory64EndSignature  = 0x06064b50
	zipDirectory64LocSignature  = 0x07064b50
	zipDirectoryHeaderLen       = 46
	zipDirectoryEndLen          = 22
	zipDirectory64EndLen        = 56
	zipDirectory64LocLen        = 20
	zipDirectorySearchLen       = 65 << 10
	zipUint16Max                = 1<<16 - 1
	zipUint32Max                = 1<<32 - 1
)

var (
	errArchiveLimit         = errors.New("archive resource limit exceeded")
	errArchiveDuplicatePath = errors.New("archive contains duplicate file path")
)

func cmdInspect(args []string) {
	enableInspectGC()

	fs := flag.NewFlagSet("brief inspect", flag.ExitOnError)
	jsonFlag := fs.Bool("json", false, "Force JSON output")
	humanFlag := fs.Bool("human", false, "Force human-readable output")
	_ = fs.Parse(args)

	if fs.NArg() == 0 {
		_, _ = fmt.Fprintln(os.Stderr, "usage: brief inspect <archive-or-object>")
		os.Exit(1)
	}

	art, err := inspectPath(fs.Arg(0))
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if *jsonFlag || (!*humanFlag && !isTTY()) {
		writeJSONOrExit(report.ArtifactJSON(os.Stdout, art))
	} else {
		report.ArtifactHuman(os.Stdout, art)
	}
}

func enableInspectGC() {
	debug.SetGCPercent(inspectGCPercent)
}

// inspectPath opens path, decides whether it is a bare native object or an
// archive, and returns an Artifact describing its native-object contents.
func inspectPath(path string) (*brief.Artifact, error) {
	start := time.Now()

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	head, err := sniff(f)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if head.Format == "" && isPEFile(f, info.Size()) {
		head.Format = magic.FormatPE
	}

	art := &brief.Artifact{
		Version: brief.Version,
		Path:    path,
		Format:  head.Format,
	}

	switch {
	case isNativeObject(head.Format):
		obj, err := binary.InspectReader(f, info.Size())
		if err != nil {
			return nil, err
		}
		obj.Path = path
		art.NativeObjects = []binary.Object{*obj}
		art.SHA256 = hashFile(f)

	case isArchive(head.Format):
		if err := inspectArchive(f, art); err != nil {
			return nil, err
		}

	default:
		return nil, fmt.Errorf("%s: not a native object or supported archive (detected %q)",
			path, magicLabel(head))
	}

	art.DurationMS = float64(time.Since(start).Microseconds()) / 1000.0 //nolint:mnd
	return art, nil
}

// inspectArchive opens f as an archive, extracts it to a temp directory, and
// walks the tree collecting native objects into art.
func inspectArchive(f *os.File, art *brief.Artifact) error {
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if err := checkArchiveInputSize(info.Size()); err != nil {
		return err
	}
	if err := preflightArtifactArchive(f, art.Path, art.Format, maxArchiveEntries, maxArchiveExtractedBytes); err != nil {
		return err
	}
	dir, err := os.MkdirTemp("", "brief-inspect-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	caseInsensitive, err := filesystemCaseInsensitive(dir)
	if err != nil {
		return err
	}

	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	r, err := openArtifactArchive(f, art.Path, art.Format, caseInsensitive)
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }()

	if h, err := r.Hash("SHA256"); err == nil {
		art.SHA256 = h
	}

	if err := archives.ExtractAll(r, dir); err != nil {
		return fmt.Errorf("extracting archive: %w", err)
	}
	if err := makeExtractedTreeAccessible(dir); err != nil {
		return fmt.Errorf("preparing extracted archive: %w", err)
	}

	return collectNativeObjects(dir, art)
}

func checkArchiveInputSize(size int64) error {
	if size < 0 || size > maxArchiveInputBytes {
		return fmt.Errorf("%w: input is %d bytes, limit is %d",
			errArchiveLimit, size, maxArchiveInputBytes)
	}
	return nil
}

type archiveInputLimitReader struct {
	io.Reader
	remaining int64
}

func newArchiveInputLimitReader(r io.Reader, maxBytes int64) *archiveInputLimitReader {
	return &archiveInputLimitReader{Reader: r, remaining: maxBytes}
}

func (r *archiveInputLimitReader) Read(p []byte) (int, error) {
	return readWithArchiveLimit(r.Reader, &r.remaining, p)
}

// preflightArtifactArchive counts entries before archives.Open eagerly reads
// and indexes them. This keeps the entry cap effective for hostile archives
// with a small payload and a very large number of headers.
func preflightArtifactArchive(f *os.File, filePath, format string, maxEntries int, maxBytes int64) error {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}

	switch format {
	case magic.FormatZIP:
		info, err := f.Stat()
		if err != nil {
			return err
		}
		return preflightZIP(f, info.Size(), maxEntries)
	case magic.FormatTAR:
		r := newArchiveInputLimitReader(f, maxArchiveInputBytes)
		if strings.EqualFold(filepath.Ext(filePath), ".gem") {
			return preflightGem(r, maxEntries, maxBytes)
		}
		return preflightTAR(r, maxEntries, maxBytes)
	case magic.FormatGZIP:
		gz, err := gzip.NewReader(newArchiveInputLimitReader(f, maxArchiveInputBytes))
		if err != nil {
			return fmt.Errorf("opening gzip: %w", err)
		}
		defer func() { _ = gz.Close() }()
		return preflightTAR(gz, maxEntries, maxBytes)
	case magic.FormatBZIP2:
		return preflightTAR(bzip2.NewReader(newArchiveInputLimitReader(f, maxArchiveInputBytes)), maxEntries, maxBytes)
	case magic.FormatXZ:
		if err := preflightXZ(newArchiveInputLimitReader(f, maxArchiveInputBytes), maxXZDictionaryBytes); err != nil {
			return err
		}
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return err
		}
		xzReader, err := xz.NewReader(newArchiveInputLimitReader(f, maxArchiveInputBytes))
		if err != nil {
			return fmt.Errorf("opening xz: %w", err)
		}
		return preflightTAR(xzReader, maxEntries, maxBytes)
	default:
		return nil
	}
}

func preflightTAR(r io.Reader, maxEntries int, maxBytes int64) error {
	tr := tar.NewReader(r)
	entries := 0
	var total int64
	for {
		header, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("reading tar: %w", err)
		}
		if err := checkTARHeader(header, &entries, &total, maxEntries, maxBytes); err != nil {
			return err
		}
	}
}

func checkTARHeader(header *tar.Header, entries *int, total *int64, maxEntries int, maxBytes int64) error {
	(*entries)++
	if *entries > maxEntries {
		return fmt.Errorf("%w: more than %d entries", errArchiveLimit, maxEntries)
	}

	mode := header.FileInfo().Mode()
	if header.Typeflag == tar.TypeLink {
		mode |= fs.ModeIrregular
	}
	if header.Typeflag == tar.TypeDir || mode&fs.ModeType != 0 {
		return nil
	}
	if header.Size < 0 || header.Size > maxBytes-(*total) {
		return fmt.Errorf("%w: declared content exceeds %d bytes", errArchiveLimit, maxBytes)
	}
	*total += header.Size
	return nil
}

// preflightGem checks the nested data.tar.gz that archives.Open exposes. If
// the nested payload is malformed, the caller retains the existing behaviour
// of treating the outer file as a plain tar archive.
func preflightGem(r io.Reader, maxEntries int, maxBytes int64) error {
	tr := tar.NewReader(r)
	entries := 0
	var total int64
	for {
		header, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("reading gem tar: %w", err)
		}
		if err := checkTARHeader(header, &entries, &total, maxEntries, maxBytes); err != nil {
			return err
		}
		if header.Name != "data.tar.gz" {
			continue
		}

		gz, err := gzip.NewReader(tr)
		if err != nil {
			continue
		}
		innerErr := preflightTAR(gz, maxEntries, maxBytes)
		_ = gz.Close()
		if innerErr == nil || errors.Is(innerErr, errArchiveLimit) {
			return innerErr
		}
	}
}

type zipDirectoryEnd struct {
	offset           int64
	directoryRecords uint64
	directorySize    uint64
	directoryOffset  uint64
}

func preflightZIP(r io.ReaderAt, size int64, maxEntries int) error {
	end, err := readZIPDirectoryEnd(r, size)
	if err != nil {
		return fmt.Errorf("reading zip directory: %w", err)
	}
	if end.directoryRecords > uint64(maxEntries) {
		return fmt.Errorf("%w: %d entries exceeds limit of %d",
			errArchiveLimit, end.directoryRecords, maxEntries)
	}

	maxInt64 := uint64(1<<63 - 1)
	if end.directorySize > maxInt64 || end.directoryOffset > maxInt64 {
		return errors.New("zip directory offset exceeds supported size")
	}
	if end.directorySize > uint64(end.offset) {
		return errors.New("zip directory size exceeds the archive")
	}
	start := end.offset - int64(end.directorySize)
	if start < 0 || start >= size && end.directorySize != 0 {
		return errors.New("zip directory offset is outside the archive")
	}

	// Match archive/zip's compatibility path for files whose end record gives
	// a spurious positive base offset but whose raw directory offset is valid.
	if end.directoryOffset < uint64(start) && int64(end.directoryOffset) < size {
		var signature [4]byte
		if _, readErr := r.ReadAt(signature[:], int64(end.directoryOffset)); readErr == nil &&
			stdbin.LittleEndian.Uint32(signature[:]) == zipDirectoryHeaderSignature {
			start = int64(end.directoryOffset)
		}
	}

	var header [zipDirectoryHeaderLen]byte
	entries := 0
	for pos := start; pos+zipDirectoryHeaderLen <= size; {
		if _, err := r.ReadAt(header[:], pos); err != nil {
			return fmt.Errorf("reading zip directory entry: %w", err)
		}
		if stdbin.LittleEndian.Uint32(header[:4]) != zipDirectoryHeaderSignature {
			break
		}
		entries++
		if entries > maxEntries {
			return fmt.Errorf("%w: more than %d entries", errArchiveLimit, maxEntries)
		}

		nameLen := int64(stdbin.LittleEndian.Uint16(header[28:30]))
		extraLen := int64(stdbin.LittleEndian.Uint16(header[30:32]))
		commentLen := int64(stdbin.LittleEndian.Uint16(header[32:34]))
		next := pos + zipDirectoryHeaderLen + nameLen + extraLen + commentLen
		if next <= pos || next > size {
			return errors.New("zip directory entry extends beyond the archive")
		}
		pos = next
	}
	return nil
}

func readZIPDirectoryEnd(r io.ReaderAt, size int64) (zipDirectoryEnd, error) {
	if size < zipDirectoryEndLen {
		return zipDirectoryEnd{}, io.ErrUnexpectedEOF
	}
	searchLen := int64(zipDirectorySearchLen + zipDirectoryEndLen)
	if searchLen > size {
		searchLen = size
	}
	buf := make([]byte, int(searchLen))
	if _, err := r.ReadAt(buf, size-searchLen); err != nil && err != io.EOF {
		return zipDirectoryEnd{}, err
	}

	index := -1
	for i := len(buf) - zipDirectoryEndLen; i >= 0; i-- {
		if stdbin.LittleEndian.Uint32(buf[i:i+4]) != zipDirectoryEndSignature {
			continue
		}
		commentLen := int(stdbin.LittleEndian.Uint16(buf[i+20 : i+22]))
		if i+zipDirectoryEndLen+commentLen <= len(buf) {
			index = i
			break
		}
	}
	if index < 0 {
		return zipDirectoryEnd{}, errors.New("zip end record not found")
	}

	record := buf[index : index+zipDirectoryEndLen]
	if stdbin.LittleEndian.Uint16(record[4:6]) != 0 || stdbin.LittleEndian.Uint16(record[6:8]) != 0 {
		return zipDirectoryEnd{}, errors.New("multi-disk zip archives are unsupported")
	}
	end := zipDirectoryEnd{
		offset:           size - searchLen + int64(index),
		directoryRecords: uint64(stdbin.LittleEndian.Uint16(record[10:12])),
		directorySize:    uint64(stdbin.LittleEndian.Uint32(record[12:16])),
		directoryOffset:  uint64(stdbin.LittleEndian.Uint32(record[16:20])),
	}

	needsZIP64 := end.directoryRecords == zipUint16Max || end.directorySize == zipUint32Max || end.directoryOffset == zipUint32Max
	if !needsZIP64 {
		return end, nil
	}
	zip64End, found, err := readZIP64DirectoryEnd(r, end)
	if err != nil {
		return zipDirectoryEnd{}, err
	}
	if found {
		return zip64End, nil
	}
	if end.directorySize == zipUint32Max || end.directoryOffset == zipUint32Max {
		return zipDirectoryEnd{}, errors.New("zip64 locator not found")
	}
	return end, nil
}

func readZIP64DirectoryEnd(r io.ReaderAt, end zipDirectoryEnd) (zipDirectoryEnd, bool, error) {
	locatorOffset := end.offset - zipDirectory64LocLen
	if locatorOffset < 0 {
		return end, false, nil
	}
	var locator [zipDirectory64LocLen]byte
	if _, err := r.ReadAt(locator[:], locatorOffset); err != nil {
		return zipDirectoryEnd{}, false, err
	}
	if stdbin.LittleEndian.Uint32(locator[:4]) != zipDirectory64LocSignature {
		return end, false, nil
	}
	if stdbin.LittleEndian.Uint32(locator[4:8]) != 0 ||
		stdbin.LittleEndian.Uint32(locator[16:20]) != 1 {
		return zipDirectoryEnd{}, false, errors.New("invalid zip64 locator")
	}

	zip64Offset := stdbin.LittleEndian.Uint64(locator[8:16])
	if zip64Offset > uint64(1<<63-1) {
		return zipDirectoryEnd{}, false, errors.New("zip64 directory offset exceeds supported size")
	}
	var record [zipDirectory64EndLen]byte
	if _, err := r.ReadAt(record[:], int64(zip64Offset)); err != nil {
		return zipDirectoryEnd{}, false, err
	}
	if stdbin.LittleEndian.Uint32(record[:4]) != zipDirectory64EndSignature ||
		stdbin.LittleEndian.Uint32(record[16:20]) != 0 ||
		stdbin.LittleEndian.Uint32(record[20:24]) != 0 {
		return zipDirectoryEnd{}, false, errors.New("invalid zip64 end record")
	}

	end.offset = int64(zip64Offset)
	end.directoryRecords = stdbin.LittleEndian.Uint64(record[32:40])
	end.directorySize = stdbin.LittleEndian.Uint64(record[40:48])
	end.directoryOffset = stdbin.LittleEndian.Uint64(record[48:56])
	return end, true, nil
}

// openArtifactArchive uses the sniffed physical format instead of trusting a
// possibly misleading filename extension. Gems need their filename for the
// archives package to unwrap data.tar.gz; malformed gems fall back to plain
// tar inspection.
func openArtifactArchive(f *os.File, path, format string, caseInsensitive bool) (*archiveLimitReader, error) {
	var r archives.Reader
	var err error
	if format == magic.FormatTAR && strings.EqualFold(filepath.Ext(path), ".gem") {
		r, err = archives.Open(filepath.Base(path), newArchiveInputLimitReader(f, maxArchiveInputBytes))
		if err != nil {
			r = nil
			if _, seekErr := f.Seek(0, io.SeekStart); seekErr != nil {
				return nil, seekErr
			}
		}
	}
	if r == nil {
		r, err = archives.Open("", newArchiveInputLimitReader(f, maxArchiveInputBytes))
		if err != nil {
			return nil, fmt.Errorf("opening archive: %w", err)
		}
	}

	limited, err := newArchiveLimitReader(r, maxArchiveEntries, maxArchiveExtractedBytes, caseInsensitive)
	if err != nil {
		_ = r.Close()
		return nil, err
	}
	return limited, nil
}

type archiveLimitReader struct {
	archives.Reader
	entries   []archives.FileInfo
	remaining int64
}

func newArchiveLimitReader(
	r archives.Reader,
	maxEntries int,
	maxBytes int64,
	caseInsensitive bool,
) (*archiveLimitReader, error) {
	entries, err := r.List()
	if err != nil {
		return nil, err
	}
	if len(entries) > maxEntries {
		return nil, fmt.Errorf("%w: %d entries exceeds limit of %d",
			errArchiveLimit, len(entries), maxEntries)
	}
	if err := checkDuplicateArchivePaths(entries, caseInsensitive); err != nil {
		return nil, err
	}

	var total int64
	for _, entry := range entries {
		if entry.IsDir || fs.FileMode(entry.Mode)&fs.ModeType != 0 {
			continue
		}
		if entry.Size < 0 || entry.Size > maxBytes-total {
			return nil, fmt.Errorf("%w: declared content exceeds %d bytes",
				errArchiveLimit, maxBytes)
		}
		total += entry.Size
	}
	entries = accessibleArchiveEntries(entries)

	return &archiveLimitReader{
		Reader:    r,
		entries:   entries,
		remaining: maxBytes,
	}, nil
}

func accessibleArchiveEntries(entries []archives.FileInfo) []archives.FileInfo {
	accessible := append([]archives.FileInfo(nil), entries...)
	for i := range accessible {
		entry := &accessible[i]
		if !entry.IsDir || !entry.HasMode {
			continue
		}
		entry.Mode = uint32(fs.FileMode(entry.Mode) | archiveDirectoryAccess)
	}
	return accessible
}

func checkDuplicateArchivePaths(entries []archives.FileInfo, caseInsensitive bool) error {
	seen := make(map[string]string)
	for _, entry := range entries {
		if entry.IsDir || fs.FileMode(entry.Mode)&fs.ModeType != 0 {
			continue
		}
		name := pathpkg.Clean(strings.TrimSuffix(entry.Path, "/"))
		if name == "." || name == "" {
			continue
		}
		local, err := filepath.Localize(name)
		if err != nil {
			// ExtractAll reports the unsafe path with its established error.
			continue
		}
		key := filepath.Clean(local)
		if caseInsensitive {
			key = strings.ToLower(key)
		}
		if previous, ok := seen[key]; ok {
			return fmt.Errorf("%w: %q conflicts with %q",
				errArchiveDuplicatePath, entry.Path, previous)
		}
		seen[key] = entry.Path
	}
	return nil
}

func filesystemCaseInsensitive(dir string) (bool, error) {
	f, err := os.CreateTemp(dir, "brief-case-probe-a")
	if err != nil {
		return false, err
	}
	name := f.Name()
	if err := f.Close(); err != nil {
		_ = os.Remove(name)
		return false, err
	}
	defer func() { _ = os.Remove(name) }()

	upper := filepath.Join(dir, strings.ToUpper(filepath.Base(name)))
	if upper == name {
		return false, errors.New("case-sensitivity probe did not produce a distinct path")
	}
	if _, err := os.Stat(upper); err == nil {
		return true, nil
	} else if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	} else {
		return false, err
	}
}

func makeExtractedTreeAccessible(root string) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		perm := info.Mode().Perm()
		switch {
		case d.IsDir():
			perm |= 0o700
		case info.Mode().IsRegular():
			perm |= 0o400
		default:
			return nil
		}
		if perm == info.Mode().Perm() {
			return nil
		}
		return os.Chmod(path, perm)
	})
}

func (r *archiveLimitReader) List() ([]archives.FileInfo, error) {
	return r.entries, nil
}

func (r *archiveLimitReader) Extract(path string) (io.ReadCloser, error) {
	content, err := r.Reader.Extract(path)
	if err != nil {
		return nil, err
	}
	return &archiveLimitReadCloser{
		ReadCloser: content,
		remaining:  &r.remaining,
	}, nil
}

type archiveLimitReadCloser struct {
	io.ReadCloser
	remaining *int64
}

func (r *archiveLimitReadCloser) Read(p []byte) (int, error) {
	return readWithArchiveLimit(r.ReadCloser, r.remaining, p)
}

func readWithArchiveLimit(r io.Reader, remaining *int64, p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if *remaining == 0 {
		var probe [1]byte
		n, err := r.Read(probe[:])
		if n > 0 {
			return 0, errArchiveLimit
		}
		return 0, err
	}
	if int64(len(p)) > *remaining {
		p = p[:int(*remaining)]
	}
	n, err := r.Read(p)
	*remaining -= int64(n)
	return n, err
}

// collectNativeObjects walks root and appends a binary.Object to art for each
// ELF, Mach-O, or PE file it finds. Paths on the resulting objects are made
// relative to root so they read as archive-entry paths.
func collectNativeObjects(root string, art *brief.Artifact) error {
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.Type().IsRegular() {
			return nil
		}
		art.Entries++

		f, err := os.Open(path)
		if err != nil {
			return err
		}
		info, err := f.Stat()
		if err != nil {
			_ = f.Close()
			return err
		}
		head, err := sniff(f)
		if err != nil {
			_ = f.Close()
			return err
		}
		native := isNativeObject(head.Format)
		if !native && head.Format == "" {
			native = isPEFile(f, info.Size())
		}
		_ = f.Close()
		if !native {
			return nil
		}

		obj, err := binary.Inspect(path)
		if err != nil {
			// A file with a native-object magic that the debug/*
			// parsers reject is unusual but not fatal for the
			// artifact as a whole; skip it.
			return nil //nolint:nilerr
		}
		obj.Path = archivePath(root, path)
		art.NativeObjects = append(art.NativeObjects, *obj)
		return nil
	})
	if err != nil {
		return err
	}
	sort.Slice(art.NativeObjects, func(i, j int) bool {
		return art.NativeObjects[i].Path < art.NativeObjects[j].Path
	})
	return nil
}

// hashFile returns the hex SHA-256 of f from offset 0, or "" on error.
func hashFile(f *os.File) string {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return ""
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))
}

// sniff reads up to magicSniffLen bytes from f and returns the magic
// classification. PE files whose header falls beyond this prefix are handled
// separately by isPEFile. The file position is left at the end of the prefix.
func sniff(f *os.File) (magic.Result, error) {
	var head [magicSniffLen]byte
	n, err := io.ReadFull(f, head[:])
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return magic.Result{}, err
	}
	return magic.DetectPrefix(head[:n]), nil
}

func isPEFile(f *os.File, size int64) bool {
	if size < peHeaderOffsetAt+4 {
		return false
	}
	var dos [peHeaderOffsetAt + 4]byte
	if _, err := f.ReadAt(dos[:], 0); err != nil {
		return false
	}
	if dos[0] != 'M' || dos[1] != 'Z' {
		return false
	}
	offset := int64(stdbin.LittleEndian.Uint32(dos[peHeaderOffsetAt:]))
	if offset < peHeaderOffsetAt+4 || offset > size-peSignatureLen {
		return false
	}
	var signature [peSignatureLen]byte
	if _, err := f.ReadAt(signature[:], offset); err != nil {
		return false
	}
	return bytes.Equal(signature[:], []byte{'P', 'E', 0, 0})
}

// inspectAutoArgs builds the argument slice for an auto-routed inspect call
// from cmdScan's already-parsed shared flags.
func inspectAutoArgs(jsonFlag, humanFlag bool, path string) []string {
	var args []string
	if jsonFlag {
		args = append(args, "-json")
	}
	if humanFlag {
		args = append(args, "-human")
	}
	return append(args, "--", path)
}

// shouldAutoInspect reports whether the default command should route path to
// cmdInspect: it is a regular file whose header is a native object or archive.
func shouldAutoInspect(path string) bool {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	head, err := sniff(f)
	if err != nil {
		_ = f.Close()
		return false
	}
	isArtifact := isNativeObject(head.Format) || isArchive(head.Format) || isPEFile(f, info.Size())
	_ = f.Close()
	return isArtifact
}

func isNativeObject(format string) bool {
	switch format {
	case magic.FormatELF, magic.FormatMachO, magic.FormatPE:
		return true
	}
	return false
}

func isArchive(format string) bool {
	switch format {
	case magic.FormatZIP, magic.FormatTAR, magic.FormatGZIP,
		magic.FormatBZIP2, magic.FormatXZ:
		return true
	}
	return false
}

func magicLabel(r magic.Result) string {
	if r.Format != "" {
		return r.Format
	}
	return string(r.Kind)
}

func archivePath(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(rel)
}
