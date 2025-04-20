// To know whether one owns dataPiece or not
package storage

type Bitfield []byte // len == numPieces

// Returns new bitfield sized n bytes
func NewBitfield(n int) Bitfield { return make([]byte, n) }

func (bf Bitfield) Has(i int) bool { return bf[i] == 1 }
func (bf Bitfield) Set(i int)      { bf[i] = 1 }

// Serialize to/from
func (bf Bitfield) Bytes() []byte     { return []byte(bf) }
func ParseBitfield(b []byte) Bitfield { return Bitfield(b) }
