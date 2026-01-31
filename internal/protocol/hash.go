// Package protocol defines the BitTorrent message structures and encoding.
package protocol

import (
	"crypto/sha1" // nolint:gosec // SHA-1 is required by the BitTorrent protocol
	"os"
)

// InfoHash reads a torrent file from disk and returns the SHA-1 20-byte ID
// that all peers must present in their Handshake.
func InfoHash(path string) ([20]byte, error) {
	// nolint:gosec // path is provided by user via CLI
	data, err := os.ReadFile(path)
	if err != nil {
		return [20]byte{}, err
	}

	// nolint:gosec // SHA-1 is required by the BitTorrent protocol
	return sha1.Sum(data), nil
}
