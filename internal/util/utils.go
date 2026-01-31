// Package util provides common utility functions.
package util

import (
	"encoding/binary"
	"os"
)

// Exists checks if a file exists at the given path.
func Exists(path string) bool {
	_, err := os.Stat(path)

	return err == nil
}

// Uint32ToBytes translates a 4-byte int to raw bytes.
func Uint32ToBytes(n uint32) []byte {
	var bytes [4]byte
	binary.BigEndian.PutUint32(bytes[:], n)

	return bytes[:]
}

// Sha1Sum takes a byte slice and returns a 20-byte array.
func Sha1Sum(bytes []byte) [20]byte {
	var arr [20]byte
	copy(arr[:], bytes)

	return arr
}
