package storage

import (
	"crypto/sha1"
	"io"
	"os"
)

// DefaultPiece is the default piece size (256 KiB).
const DefaultPiece = 256 * 1024

// Split reads the file at path and returns slices with data of each piece
// along with an array of SHA-1 hashes (20 bytes per piece).
func Split(path string, pieceSize int) (pieces [][]byte, hashes [][]byte, err error) {
	if pieceSize <= 0 {
		pieceSize = DefaultPiece
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}

	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	buf := make([]byte, pieceSize)
	for {
		n, readErr := io.ReadFull(f, buf)

		switch {
		case readErr == io.EOF || readErr == io.ErrUnexpectedEOF:
			if n == 0 {
				goto done
			}
			// Getting final partial piece
			p := make([]byte, n)
			copy(p, buf[:n])
			pieces = append(pieces, p)

			if readErr == io.EOF {
				goto done
			}
		case readErr != nil:
			return nil, nil, readErr
		default:
			// We are getting a full piece
			p := make([]byte, pieceSize)
			copy(p, buf)
			pieces = append(pieces, p)
		}
	}

done:
	// Calculate hashes
	for _, p := range pieces {
		h := sha1.Sum(p)
		hashCopy := h[:] // Convert [20]byte into -> []byte
		hashes = append(hashes, hashCopy)
	}

	return pieces, hashes, nil
}
