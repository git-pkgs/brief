package brief

import "github.com/git-pkgs/brief/binary"

// Artifact is the output of inspecting a distributed package archive or a
// bare native object file.
type Artifact struct {
	Version string `json:"version"`
	Path    string `json:"path"`

	// Format is the physical container format as reported by
	// git-pkgs/magic: "zip", "gzip", "tar", "elf", "mach-o", "pe".
	// Packaging-level identity (wheel, gem, jar) is added by later
	// container metadata parsing.
	Format string `json:"format"`

	// SHA256 is the hex-encoded digest of the raw input file.
	SHA256 string `json:"sha256,omitempty"`

	// Entries is the total number of regular-file entries walked when the
	// input is an archive. Zero for bare native objects.
	Entries int `json:"entries,omitempty"`

	// NativeObjects lists every ELF, Mach-O, or PE object found: the input
	// itself when it is a bare native object, or each such entry inside an
	// archive.
	NativeObjects []binary.Object `json:"native_objects,omitempty"`

	// DurationMS is wall-clock milliseconds spent producing the report,
	// excluding archive download time.
	DurationMS float64 `json:"duration_ms"`
}
