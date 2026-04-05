//go:build unix

package detect

import (
	"os"
	"syscall"
)

// openNoFollow opens a file with O_NOFOLLOW so the kernel rejects
// symlinks, preventing TOCTOU races between Lstat and Open.
func openNoFollow(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
}
