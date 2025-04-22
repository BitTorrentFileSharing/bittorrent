// To know whether one owns dataPiece or not
package storage

// Just a bunch of bytes
type Bitfield []byte // len == numPieces

// Returns new bitfield sized n bytes.
// Bitfield corresponds to piece ownership fact
func NewBitfield(n int) Bitfield { return make([]byte, n) }

// Asks i-th == 1?
func (bf Bitfield) Has(i int) bool { return bf[i] == 1 }
// Sets i-th bit to 1
func (bf Bitfield) Set(i int)      { bf[i] = 1 }

// Serialize bitfield to bytes
func (bf Bitfield) Bytes() []byte     { return []byte(bf) }
// Parse bytes for bitfield
func ParseBitfield(b []byte) Bitfield { return Bitfield(b) }
