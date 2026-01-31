package protocol

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"io"
	"math"

	"github.com/BitTorrentFileSharing/bittorrent/internal/bitutil"
	"github.com/BitTorrentFileSharing/bittorrent/internal/storage"
)

// Message type constants.
const (
	MsgHandshake = iota
	MsgBitfield
	MsgRequest
	MsgPiece
	MsgHave
)

const (
	idSize       = 20
	handshakeLen = 1 + idSize + idSize
	uint32Size   = 4
)

// Message represents a protocol message with ID and data payload.
type Message struct {
	ID   uint8
	Data []byte
}

// NewHandshake creates a new handshake message.
func NewHandshake(infoHash, peerID []byte) Message {
	return Message{ID: MsgHandshake, Data: append(infoHash, peerID...)}
}

// NewBitfield creates a new bitfield message.
func NewBitfield(bf storage.Bitfield) Message {
	return Message{ID: MsgBitfield, Data: bf.Bytes()}
}

// NewRequest creates a new request message for the given piece index.
func NewRequest(idx int) Message {
	// nolint:gosec // idx is assumed to be within uint32 range for BitTorrent
	index := uint32(idx)

	return Message{
		ID: MsgRequest,
		Data: append(
			bitutil.Uint32ToBytes(index),
			bitutil.Uint32ToBytes(0)..., // Offset is 0
		),
	}
}

// NewPiece creates a new piece message with the given index and data.
func NewPiece(idx int, piece []byte) Message {
	// nolint:gosec // idx is assumed to be within uint32 range for BitTorrent
	index := uint32(idx)

	return Message{
		ID: MsgPiece,
		Data: append(
			append(
				bitutil.Uint32ToBytes(index),
				bitutil.Uint32ToBytes(0)..., // Offset is 0
			),
			piece...,
		),
	}
}

// NewHave creates a new have message for the given piece index.
func NewHave(idx int) Message {
	// nolint:gosec // idx is assumed to be within uint32 range for BitTorrent
	index := uint32(idx)

	return Message{
		ID:   MsgHave,
		Data: bitutil.Uint32ToBytes(index),
	}
}

// Encode forms a TCP packet and writes it to the given writer.
func (m *Message) Encode(pipe io.Writer) error {
	payloadLen := 1 + len(m.Data)
	if payloadLen > math.MaxUint32 {
		return errors.New("message too large")
	}

	// 1. Write prefix which tells length of message.
	// nolint:gosec // payloadLen is checked against math.MaxUint32 above
	if err := binary.Write(pipe, binary.BigEndian, uint32(payloadLen)); err != nil {
		return err
	}
	// 2. writes type of msg (1-byte ID).
	if err := binary.Write(pipe, binary.BigEndian, m.ID); err != nil {
		return err
	}
	// 3. writes payload
	_, err := pipe.Write(m.Data)

	return err
}

// Decode reads and decodes a message with ID and DATA from the reader.
func Decode(r io.Reader) (*Message, error) {
	// 1. Read data length (represented in 4 bytes)
	var size uint32
	if err := binary.Read(r, binary.BigEndian, &size); err != nil {
		return nil, err
	}

	if size == 0 {
		return nil, errors.New("invalid message size")
	}
	// 2. Read the rest of data sized [size]
	buf := make([]byte, size)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}

	return &Message{ID: buf[0], Data: buf[1:]}, nil
}

// RandomPeerID generates a random 20-byte peer ID.
func RandomPeerID() [idSize]byte {
	var id [idSize]byte

	_, err := rand.Read(id[:])
	if err != nil {
		panic(err)
	}

	return id
}
