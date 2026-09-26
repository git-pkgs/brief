//go:build tinygo

package binary

import (
	"errors"
	"fmt"
	"io"
)

func readGoBuild(r io.ReaderAt) (*GoBuild, error) {
	return nil, fmt.Errorf("brief/binary: Go buildinfo inspection not available under tinygo: %w", errors.ErrUnsupported)
}
