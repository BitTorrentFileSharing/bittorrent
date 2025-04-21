package storage

import (
	"os"
	"path/filepath"
)

// Writes multiple pieces of bytes into file
func Join(pieces [][]byte, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	file, err := os.Create(path); if err != nil {
		return err
	}
	defer file.Close()

	for _, piece := range pieces {
		if _, err := file.Write(piece); err != nil {
			return err
		}
	}
	return nil
}
