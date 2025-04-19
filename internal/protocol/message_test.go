package protocol_test

import (
	"fmt"
	"github.com/BitTorrentFileSharing/bittorrent/internal/protocol"
	"bytes"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	m := protocol.Message{ID: 5, Data: make([]byte, 6)}
	m.Data[0] = 42 // let's make the data not all zeroes just for fun

	fmt.Printf("Original: ID=%d, Data=%#v\n", m.ID, m.Data)

	var buf bytes.Buffer
	if err := m.Encode(&buf); err != nil {
		t.Fatal("encode failed:", err)
	}

	msg2, err := protocol.Decode(&buf)
	if err != nil {
		t.Fatal("decode failed:", err)
	}

	fmt.Printf("Decoded:  ID=%d, Data=%#v\n", msg2.ID, msg2.Data)

	// Optional: real test comparison
	if msg2.ID != m.ID || !bytes.Equal(msg2.Data, m.Data) {
		t.Errorf("roundtrip mismatch:\nwant: ID=%d, Data=%v\ngot:  ID=%d, Data=%v",
			m.ID, m.Data, msg2.ID, msg2.Data)
	}
}