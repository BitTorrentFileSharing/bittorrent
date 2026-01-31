package storage

import (
	"os"
	"path/filepath"
)

const dirPerms = 0o750

// Join writes multiple pieces of bytes into a single file at the given path.
func Join(pieces [][]byte, path string) (err error) {
	if err := os.MkdirAll(filepath.Dir(path), dirPerms); err != nil {
		return err
	}

	// nolint:gosec // path is provided by user via CLI
	file, err := os.Create(path)
	if err != nil {
		return err
	}

	defer func() {
		if cerr := file.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	for _, piece := range pieces {
		if _, err := file.Write(piece); err != nil {
			return err
		}
	}

	return nil
}
