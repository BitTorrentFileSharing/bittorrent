// Package storage handles bitfield management and file I/O for pieces.
package storage

import (
	"crypto/sha1" // nolint:gosec // SHA-1 is required by the BitTorrent protocol
	"io"
	"os"
)

// DefaultPiece is the standard size for a torrent piece (256 KiB).
const DefaultPiece = 256 * 1024 // 256 KiB

// Split reads the file at path and returns slices with data of each piece
// along with an array of SHA-1 hashes (20 bytes per piece).
// nolint:nonamedreturns // Named returns are used for deferred error handling
func Split(path string, pieceSize int) (pieces [][]byte, hashes [][]byte, err error) {
	if pieceSize <= 0 {
		pieceSize = DefaultPiece
	}

	// nolint:gosec // path is provided by user via CLI
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}

	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	pieces, err = readPieces(f, pieceSize)
	if err != nil {
		return nil, nil, err
	}

	hashes = calculateHashes(pieces)

	return pieces, hashes, nil
}

func readPieces(f io.Reader, pieceSize int) ([][]byte, error) {
	var pieces [][]byte

	buf := make([]byte, pieceSize)

	for {
		n, readErr := io.ReadFull(f, buf)

		switch {
		case readErr == io.EOF || readErr == io.ErrUnexpectedEOF:
			if n == 0 {
				return pieces, nil
			}

			p := make([]byte, n)
			copy(p, buf[:n])
			pieces = append(pieces, p)

			return pieces, nil
		case readErr != nil:
			return nil, readErr
		default:
			p := make([]byte, pieceSize)
			copy(p, buf)
			pieces = append(pieces, p)
		}
	}
}

func calculateHashes(pieces [][]byte) [][]byte {
	hashes := make([][]byte, 0, len(pieces))

	for _, p := range pieces {
		// nolint:gosec // SHA-1 is required by the BitTorrent protocol
		h := sha1.Sum(p)
		hashCopy := h[:] // Convert [20]byte into -> []byte
		hashes = append(hashes, hashCopy)
	}

	return hashes
}
