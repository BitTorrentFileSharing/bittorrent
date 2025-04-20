package protocol

import (
	"encoding/binary"
	"errors"
	"io"
	"math/rand"

	"github.com/BitTorrentFileSharing/bittorrent/internal/storage"
)

const (
	MsgHandshake = iota
	MsgBitfield
	MsgRequest
	MsgPiece
)

// Handshake payload:
//   20‑byte infoHash  – SHA‑1(torrent metadata)
//   20‑byte peerID    – random ASCII string
//
// length = 1 (ID) + 20 + 20 = 41
const HandshakeLen = 1 + 20 + 20

type Message struct {
	ID   uint8
	Data []byte
}

func NewHandshake(infoHash, peerID []byte) Message {
	return Message{ID: MsgHandshake, Data: append(infoHash, peerID...)}
}

func NewBitfield(bf storage.Bitfield) Message {
	return Message{ID: MsgBitfield, Data: bf.Bytes()}
}

func (m *Message) Encode(w io.Writer) error {
	// length prefix = 1 + len(data)
	if err := binary.Write(w, binary.BigEndian, uint32(1+len(m.Data))); err != nil {
		return err;
	}
	if err := binary.Write(w, binary.BigEndian, m.ID); err != nil {
		return err
	}
	_, err := w.Write(m.Data)
	return err
}

// Decodes message with ID and DATA from reader
func Decode(r io.Reader) (*Message, error) {
	var size uint32
	if err := binary.Read(r, binary.BigEndian, &size); err != nil {
		return nil, err
	}
	if size == 0 {
		return nil, errors.New("invalid message size")
	}
	buf := make([]byte, size)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	return &Message{ID: buf[0], Data: buf[1:]}, nil
}

func RandomPeerID() [20]byte {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	var b [20]byte
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return b
}
