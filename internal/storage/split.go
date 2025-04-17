package storage

import (
	"crypto/sha1"
	"io"
	"os"
)

const DefaultPiece = 256 * 1024 // 256 KiB

// Split reads *path* and returns slices with data of each piece
// with an array of SHA-1 hashes (20bytes per piece)
func Split(path string, pieceSize int) (pieces [][]byte, hashes [][]byte, err error) {
	if pieceSize <= 0 {
		pieceSize = DefaultPiece
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()

	buf := make([]byte, pieceSize)
	for {
		n, readErr := io.ReadFull(f, buf)
		if readErr == io.EOF || readErr == io.ErrUnexpectedEOF {
			if n == 0 {
				break
			}
			// Getting to this transition unit means that
			// we are getting final partial piece
			p := make([]byte, n)
			copy(p, buf[:n])
			pieces = append(pieces, p)
		} else if readErr != nil {
			return nil, nil, readErr
		} else {
			// We are getting full fucking piece
			p := make([]byte, pieceSize)
			copy(p, buf)
			pieces = append(pieces, p)
		}
	}

	// Calculate hashes
	for _, p := range pieces {
		h := sha1.Sum(p)
		hashCopy := h[:] // Convert [20]byte into -> []byte
		hashes = append(hashes, hashCopy)
	}
	return pieces, hashes, nil
}
