package protocol

import (
	"crypto/sha1"
	"os"
)

// Reads torrent file from dist and returns
// SHA-1 20-byte ID that all peers must
// present in their Handshake.
func InfoHash(path string) ([20]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return [20]byte{}, err
	}
	return sha1.Sum(data), nil
}
