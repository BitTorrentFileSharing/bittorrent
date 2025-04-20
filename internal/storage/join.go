package storage

import (
	"os"
)

// Writes pieces into file
func Join(pieces [][]byte, path string) error {
	file, err := os.Create(path)
	if err != nil {
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
