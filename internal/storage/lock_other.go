//go:build !darwin && !linux && !freebsd && !openbsd && !netbsd && !dragonfly

package storage

import (
	"fmt"
	"os"
)

func lockFile(_ *os.File) error {
	return fmt.Errorf("data-dir locking currently supports Unix platforms only")
}
