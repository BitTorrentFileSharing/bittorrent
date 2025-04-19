package protocol

import (
	"encoding/binary"
	"io"
	"errors"
)

const (
	MsgHandshake = iota
	MsgBitfield
	MsgRequest
	MsgPiece
)

// 1-byte protocol name length
// 32-byte infoHash
// 20-byte peerID
const HandshakeLen = 1 + 32 + 20

type Message struct {
	ID   uint8
	Data []byte
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

func Decode(r io.Reader) (*Message, error) {
	var size uint32
	if err := binary.Read(r, binary.BigEndian, &size); err != nil {
		return nil, err
	}
	if size < 1 {
		return nil, errors.New("invalid message size")
	}
	buf := make([]byte, size)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	return &Message{ID: buf[0], Data: buf[1:]}, nil
}
