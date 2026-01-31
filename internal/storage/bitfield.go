// Package storage provides utilities for managing torrent data pieces
// and tracking piece ownership.
package storage

// Bitfield represents piece ownership as a slice of bytes where each byte
// indicates whether a piece is owned (1) or not (0).
type Bitfield []byte // len == numPieces

// NewBitfield returns a new bitfield sized n bytes.
// Bitfield corresponds to piece ownership fact.
func NewBitfield(n int) Bitfield { return make([]byte, n) }

// Has returns true if the i-th piece is owned.
func (bf Bitfield) Has(i int) bool { return bf[i] == 1 }

// Set marks the i-th piece as owned.
func (bf Bitfield) Set(i int) { bf[i] = 1 }

// Bytes serializes the bitfield to bytes.
func (bf Bitfield) Bytes() []byte { return []byte(bf) }

// ParseBitfield parses bytes into a Bitfield.
func ParseBitfield(b []byte) Bitfield { return Bitfield(b) }
