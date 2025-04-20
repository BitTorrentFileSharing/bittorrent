package util

import (
	"os"
	"encoding/binary"
)

// Checks if file exists on this path
func Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// Translate 4-byte int to raw bytes
func Uint32ToBytes(n uint32) []byte {
	var bytes [4]byte
	binary.BigEndian.PutUint32(bytes[:], n)
	return bytes[:]
}

// Takes 20-byte sequence from argument
func Sha1Sum(bytes []byte) [20]byte {
    var arr [20]byte
    copy(arr[:], bytes)
    return arr
}
