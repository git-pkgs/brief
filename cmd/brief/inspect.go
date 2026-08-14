package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"
	"time"

	"github.com/git-pkgs/archives"
	"github.com/git-pkgs/magic"

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
)

var errArchiveLimit = errors.New("archive resource limit exceeded")

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

	art := &brief.Artifact{
		Version: brief.Version,
		Path:    path,
		Format:  head.Format,
	}

	switch {
	case isNativeObject(head.Format):
		obj, err := binary.Inspect(path)
		if err != nil {
			return nil, err
		}
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

	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	r, err := openArtifactArchive(f, art.Path, art.Format)
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }()

	if h, err := r.Hash("SHA256"); err == nil {
		art.SHA256 = h
	}

	dir, err := os.MkdirTemp("", "brief-inspect-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(dir) }()

	if err := archives.ExtractAll(r, dir); err != nil {
		return fmt.Errorf("extracting archive: %w", err)
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

// openArtifactArchive uses the sniffed physical format instead of trusting a
// possibly misleading filename extension. Gems need their filename for the
// archives package to unwrap data.tar.gz; malformed gems fall back to plain
// tar inspection.
func openArtifactArchive(f *os.File, path, format string) (*archiveLimitReader, error) {
	var r archives.Reader
	var err error
	if format == magic.FormatTAR && strings.EqualFold(filepath.Ext(path), ".gem") {
		r, err = archives.Open(filepath.Base(path), f)
		if err != nil {
			r = nil
			if _, seekErr := f.Seek(0, io.SeekStart); seekErr != nil {
				return nil, seekErr
			}
		}
	}
	if r == nil {
		r, err = archives.Open("", f)
		if err != nil {
			return nil, fmt.Errorf("opening archive: %w", err)
		}
	}

	limited, err := newArchiveLimitReader(r, maxArchiveEntries, maxArchiveExtractedBytes)
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

func newArchiveLimitReader(r archives.Reader, maxEntries int, maxBytes int64) (*archiveLimitReader, error) {
	entries, err := r.List()
	if err != nil {
		return nil, err
	}
	if len(entries) > maxEntries {
		return nil, fmt.Errorf("%w: %d entries exceeds limit of %d",
			errArchiveLimit, len(entries), maxEntries)
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

	return &archiveLimitReader{
		Reader:    r,
		entries:   entries,
		remaining: maxBytes,
	}, nil
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
	if len(p) == 0 {
		return 0, nil
	}
	if *r.remaining == 0 {
		var probe [1]byte
		n, err := r.ReadCloser.Read(probe[:])
		if n > 0 {
			return 0, errArchiveLimit
		}
		return 0, err
	}
	if int64(len(p)) > *r.remaining {
		p = p[:int(*r.remaining)]
	}
	n, err := r.ReadCloser.Read(p)
	*r.remaining -= int64(n)
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
		head, err := sniff(f)
		_ = f.Close()
		if err != nil {
			return err
		}
		if !isNativeObject(head.Format) {
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
// classification. The file position is left at the end of the sniffed prefix.
func sniff(f *os.File) (magic.Result, error) {
	var head [magicSniffLen]byte
	n, err := io.ReadFull(f, head[:])
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return magic.Result{}, err
	}
	return magic.DetectPrefix(head[:n]), nil
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
	_ = f.Close()
	if err != nil {
		return false
	}
	return isNativeObject(head.Format) || isArchive(head.Format)
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
