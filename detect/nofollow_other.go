//go:build !unix

package detect

import "os"

// openNoFollow is a best-effort fallback on non-unix platforms where
// O_NOFOLLOW is not available. The Lstat check in safeReadFile still
// provides protection, just without the TOCTOU guarantee.
func openNoFollow(path string) (*os.File, error) {
	return os.Open(path)
}
