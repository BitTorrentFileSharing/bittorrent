package protocol

import (
	"encoding/binary"
	"errors"
	"io"
	"crypto/rand"

	"github.com/BitTorrentFileSharing/bittorrent/internal/storage"
	"github.com/BitTorrentFileSharing/bittorrent/internal/util"
)

const (
	MsgHandshake = iota
	MsgBitfield
	MsgRequest
	MsgPiece
	MsgHave
)

// Handshake payload:
//
//	20‑byte infoHash  – SHA‑1(torrent metadata)
//	20‑byte peerID    – random ASCII string
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

func NewRequest(idx int) Message {
	return Message{
		ID: MsgRequest,
		Data: append(
			util.Uint32ToBytes(uint32(idx)),
			util.Uint32ToBytes(0)..., // Offset is 0
		),
	}
}

func NewPiece(idx int, piece []byte) Message {
	return Message{
		ID: MsgPiece,
		Data: append(
			append(
				util.Uint32ToBytes(uint32(idx)),
				util.Uint32ToBytes(0)..., // Offset is 0
			),
			piece...,
		),
	}
}

func NewHave(idx int) Message {
	return Message{
		ID: MsgHave,
		Data: util.Uint32ToBytes(uint32(idx)),
	}
}

// Forms TCP-packet
func (m *Message) Encode(pipe io.Writer) error {
	// 1. Write prefix which tells length of message.
	// 1-byte for ID. N-byte for Data
	if err := binary.Write(pipe, binary.BigEndian, uint32(1+len(m.Data))); err != nil {
		return err
	}
	// 2. writes type of msg (1-byte ID). See translation above
	if err := binary.Write(pipe, binary.BigEndian, m.ID); err != nil {
		return err
	}
	// 3. writes payload
	_, err := pipe.Write(m.Data)
	return err
}

// Decodes message with ID and DATA from reader
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

func RandomPeerID() [20]byte {
	var id [20]byte
	_, err := rand.Read(id[:])
	if err != nil {
		panic(err)
	}
	return id
}
